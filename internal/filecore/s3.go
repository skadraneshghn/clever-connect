package filecore

import (
	"context"
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

// InitStorageCore bootstraps the S3-compatible object storage connection.
//
// It configures a tuned HTTP transport (large idle-connection pool, HTTP/2
// enabled by the Go net/http default) to prevent socket starvation under
// heavy parallel multipart traffic, points the client at the custom Cellar
// endpoint with path-style addressing, and verifies (auto-creates) the
// target bucket so the rest of the system can upload immediately.
//
// This function MUST be called after the database has been initialized.
func InitStorageCore(host, keyID, secret, bucket, region string) error {
	if host == "" || keyID == "" || secret == "" {
		logger.Info("FileCore", "S3 object storage disabled — missing CELLAR_ADDON_* credentials, using local disk")
		return nil
	}

	// Optimized transport: a generous keep-alive connection pool keeps parallel
	// multipart part uploads from churning TCP connections to the Cellar host.
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          500,
			MaxIdleConnsPerHost:   100,
			MaxConnsPerHost:       0,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
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
		// Cellar is addressed with path-style URLs:
		//   https://<host>/<bucket>/<key>
		o.BaseEndpoint = aws.String("https://" + host)
		o.UsePathStyle = true
		// Cellar (Scality S3) is NOT compatible with the AWS SDK v2 default
		// trailing-checksum behavior, which sends
		// "x-amz-content-sha256: STREAMING-UNSIGNED-PAYLOAD-TRAILER" and a CRC32
		// trailer per part — Cellar rejects this with
		// "XAmzContentSHA256Mismatch". Disabling automatic request checksum
		// calculation and response checksum validation reverts the client to
		// the classic, broadly-compatible signing path.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	engine = &StorageEngine{
		S3Client:      s3Client,
		PresignClient: s3.NewPresignClient(s3Client),
		BucketName:    bucket,
	}

	if err := ensureBucket(context.Background(), bucket); err != nil {
		// Bucket verification failed — log but keep the engine. Individual
		// uploads will surface the concrete error, and the caller can retry.
		logger.Error("FileCore", "S3 bucket verification/creation failed — uploads will error until resolved",
			"bucket", bucket, "error", err)
		engine = nil
		return err
	}

	logger.Info("FileCore", "S3 object storage engine initialized",
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
