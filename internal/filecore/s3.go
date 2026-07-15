package filecore

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"time"

	"clever-connect/internal/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// StorageEngine wraps a high-performance S3-compatible client and a presign
// client. It is the single object-storage gateway for the whole application.
//
// The engine is intentionally nil-safe: callers may invoke the package-level
// helpers (IsS3Enabled, UploadFileToS3, StreamFromS3, ...) even when S3 was not
// configured — in that case they degrade gracefully (no-op uploads, false
// lookups) so the rest of the application keeps working on local disk.
type StorageEngine struct {
	S3Client      *s3.Client
	PresignClient *s3.PresignClient
	BucketName    string
}

// engine is the process-wide singleton storage engine. It stays nil when S3
// is not configured, which makes IsS3Enabled() trivially cheap.
var engine *StorageEngine

// InitStorageCore bootstraps the S3-compatible object storage connection tuned
// for Clever Cloud Cellar (Scality S3 / Ceph RGW family).
//
// # Why HTTP/1.1 only?
//
// Cellar verifies x-amz-content-sha256 on every UploadPart by computing the
// SHA256 of the raw received bytes and comparing against the header value.
// When the connection uses HTTP/2, the Go runtime frames the body in H2 DATA
// chunks whose byte boundaries differ from the raw content, causing the
// server-side SHA256 to diverge from the SDK-computed value → 400
// XAmzContentSHA256Mismatch.  HTTP/1.1 with Content-Length sends the exact
// bytes the SDK hashed, eliminating the mismatch permanently.
//
// # Why RequestChecksumCalculation = WhenRequired (client-level)?
//
// AWS SDK v2 ≥ v1.36 appends optional CRC32/CRC64 "flexible checksum"
// trailers by default.  Cellar rejects these unknown trailers with a 400
// error.  Setting WhenRequired suppresses them for all operations.
//
// NOTE: the manager.Uploader has its OWN RequestChecksumCalculation field
// (defaults to WhenSupported) that is separate from this client-level
// setting.  The uploader's field is what controls whether
// ChecksumAlgorithm=CRC32 is auto-set on UploadPartInput — see
// uploader.go::newUploader for the matching fix.
//
// # Real SHA256, not UNSIGNED-PAYLOAD
//
// The S3 operation middleware stack installs dynamicPayloadSigningMiddleware
// (ID "ComputePayloadHash") which sends "UNSIGNED-PAYLOAD" as
// x-amz-content-sha256 for HTTPS requests.  Cellar does NOT honour
// UNSIGNED-PAYLOAD for authenticated (non-presigned) UploadPart over HTTPS
// and returns XAmzContentSHA256Mismatch.  The uploader swaps that middleware
// back to ComputePayloadSHA256 (real digest) — see uploader.go::newUploader.
//
// This function MUST be called after the database has been initialized.
func InitStorageCore(host, keyID, secret, bucket, region string) error {
	if host == "" || keyID == "" || secret == "" {
		logger.Info("FileCore", "S3 object storage disabled — missing CELLAR_ADDON_* credentials, using local disk")
		return nil
	}

	// ─── HTTP/1.1-only transport ───────────────────────────────────────────
	// Force HTTP/1.1 by advertising only "http/1.1" in the TLS ALPN extension.
	// This prevents the TLS handshake from negotiating HTTP/2 even if Cellar
	// advertises h2 support.  HTTP/1.1 with Content-Length sends bytes exactly
	// as the SDK hashed them, satisfying Cellar's SHA256 verification.
	//
	// Connection pooling settings prevent socket starvation under the parallel
	// multipart upload workers (concurrency × parts in flight at once).
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          200,
			MaxIdleConnsPerHost:   50,
			MaxConnsPerHost:       0,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			// Disable HTTP/2 at the TLS-ALPN level — the ONLY reliable way.
			// Setting ForceAttemptHTTP2: false alone is insufficient when the
			// server also advertises "h2" in its ALPN; NextProtos overrides it.
			TLSClientConfig: &tls.Config{
				NextProtos: []string{"http/1.1"},
			},
		},
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithHTTPClient(httpClient),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(keyID, secret, "")),
		config.WithRegion(region),
	)
	if err != nil {
		return errors.New("storage engine config error: " + err.Error())
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// Cellar uses path-style URLs: https://<host>/<bucket>/<key>
		o.BaseEndpoint = aws.String("https://" + host)
		o.UsePathStyle = true

		// Suppress the optional CRC32/CRC64 flexible-checksum trailers added
		// by SDK v2 ≥ v1.36 by default.  Cellar rejects unknown trailing
		// headers with a 400 error.  WhenRequired means "only send a checksum
		// when the specific S3 operation explicitly requires one", which covers
		// zero operations in the normal upload/download flow.
		//
		// NOTE: this is the CLIENT-level setting.  The manager.Uploader has
		// its own RequestChecksumCalculation field (see uploader.go) that
		// independently controls whether ChecksumAlgorithm=CRC32 is auto-set.
		// Both must be WhenRequired to prevent aws-chunked trailing checksums.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	engine = &StorageEngine{
		S3Client:      s3Client,
		PresignClient: s3.NewPresignClient(s3Client),
		BucketName:    bucket,
	}

	if err := ensureBucket(context.Background(), bucket); err != nil {
		logger.Error("FileCore", "S3 bucket verification/creation failed — uploads will error until resolved",
			"bucket", bucket, "error", err)
		engine = nil
		return err
	}

	logger.Info("FileCore", "S3 object storage engine initialized (HTTP/1.1, real-SHA256 mode)",
		"host", host, "bucket", bucket, "region", region)
	return nil
}


// ensureBucket verifies the configured bucket exists and creates it on demand.
// Creating a bucket that already exists is a safe no-op on S3-compatible stores.
func ensureBucket(ctx context.Context, bucket string) error {
	if engine == nil {
		return errors.New("storage engine not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := engine.S3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err == nil {
		return nil
	}

	logger.Warn("FileCore", "S3 bucket not found, attempting auto-creation", "bucket", bucket, "head_error", err)

	_, err = engine.S3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		// Some S3-compatible stores (incl. Cellar) return a non-200 for an
		// already-existing bucket; treat "BucketAlreadyOwnedByYou" as success.
		if strings.Contains(err.Error(), "BucketAlready") {
			return nil
		}
		return err
	}
	logger.Info("FileCore", "S3 bucket created", "bucket", bucket)
	return nil
}

// IsS3Enabled reports whether object storage is configured and available.
// It is safe to call from any goroutine at any time.
func IsS3Enabled() bool {
	return engine != nil
}

// Engine returns the process-wide storage engine, or nil if disabled.
func Engine() *StorageEngine {
	return engine
}

// PresignGetURL generates a short-lived presigned GET URL for an object key.
// The URL lets the client download directly from Cellar, offloading all
// bandwidth from the application container. Returns "" when S3 is disabled.
func PresignGetURL(ctx context.Context, key string, lifetime time.Duration) (string, error) {
	if engine == nil {
		return "", nil
	}
	if key == "" {
		return "", errors.New("empty object key")
	}
	presigned, err := engine.PresignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(engine.BucketName),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(lifetime))
	if err != nil {
		return "", err
	}
	return presigned.URL, nil
}

// PresignDownloadURL is like PresignGetURL but forces an attachment download
// with a friendly filename via the S3 response-content-disposition override.
// This makes the 302-redirect download path produce the right client-side name
// without the application proxying any bytes. Returns "" when S3 is disabled.
func PresignDownloadURL(ctx context.Context, key, filename string, lifetime time.Duration) (string, error) {
	if engine == nil {
		return "", nil
	}
	if key == "" {
		return "", errors.New("empty object key")
	}
	in := &s3.GetObjectInput{
		Bucket: aws.String(engine.BucketName),
		Key:    aws.String(key),
	}
	if filename != "" {
		in.ResponseContentDisposition = aws.String(`attachment; filename="` + filename + `"`)
	}
	presigned, err := engine.PresignClient.PresignGetObject(ctx, in, s3.WithPresignExpires(lifetime))
	if err != nil {
		return "", err
	}
	return presigned.URL, nil
}

// objectExists performs a cheap HEAD request to verify an object is present.
func objectExists(ctx context.Context, key string) bool {
	if engine == nil || key == "" {
		return false
	}
	_, err := engine.S3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(engine.BucketName),
		Key:    aws.String(key),
	})
	return err == nil
}
