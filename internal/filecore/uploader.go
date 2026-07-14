package filecore

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"

	"clever-connect/internal/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Tuned multipart defaults for Clever Cloud Cellar:
//
//   - 64 MiB parts: reduces round-trip count from ~2 000 to ~160 for a 10 GiB
//     file, which is the dominant bottleneck on a high-bandwidth link.  S3 allows
//     up to 5 GiB per part; 64 MiB is a sweet spot between latency and RAM.
//   - 16 concurrent workers: fully saturates Cellar's multi-Gbps interconnect
//     without exhausting the container's goroutine budget.
//   - Pool buffer matches part size so the SDK never allocates on the hot path.
const (
	defaultPartSize    = 64 * 1024 * 1024 // 64 MiB — optimal for large video/archive files
	minimumConcurrency = 16
	uploadBufPartSize  = 64 * 1024 * 1024 // must match defaultPartSize
)

// concurrencyForUpload scales the parallel part workers with available CPU,
// floored at minimumConcurrency so even single-core containers saturate the link
// (the bottleneck is always network, not CPU).
func concurrencyForUpload() int {
	c := runtime.GOMAXPROCS(0) * 4
	if c < minimumConcurrency {
		c = minimumConcurrency
	}
	return c
}

// newUploader builds a freshly-configured parallel multipart uploader bound to
// the active storage engine. A new uploader per call keeps configuration simple
// and lets the AWS SDK manage its own goroutine pool lifetime.
func (e *StorageEngine) newUploader() *manager.Uploader {
	return manager.NewUploader(e.S3Client, func(u *manager.Uploader) {
		u.PartSize = defaultPartSize
		u.Concurrency = concurrencyForUpload()
		// Pre-allocated seekable buffers matching the part size cut GC pressure
		// and allocation latency while parts are shuffled to S3 in parallel.
		u.BufferProvider = manager.NewBufferedReadSeekerWriteToPool(uploadBufPartSize)
	})
}

// UploadStreamToS3 streams an io.Reader straight into S3 using parallel
// multipart upload. This is the hot path for the downloader: the HTTP source
// body is piped concurrently to the object store without touching local disk.
//
// Returns the object key on success. It is a no-op (returns "") when S3 is
// disabled so callers can branch freely without extra guards.
func UploadStreamToS3(ctx context.Context, key, mimeType string, body io.Reader) (string, error) {
	if engine == nil {
		return "", nil
	}
	if key == "" {
		return "", errors.New("empty object key")
	}

	contentType := mimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	uploader := engine.newUploader()
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(engine.BucketName),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

// UploadFileToS3 uploads an already-on-disk file to S3 via the parallel
// multipart uploader and returns the object key. Used by the local-disk
// downloaders (torrent / youtube / spotify / telegram / manual upload) to push
// completed files into object storage after processing.
func UploadFileToS3(ctx context.Context, key, mimeType, localPath string) (string, error) {
	if engine == nil {
		return "", nil
	}
	if key == "" {
		return "", errors.New("empty object key")
	}

	f, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	contentType := mimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	uploader := engine.newUploader()
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(engine.BucketName),
		Key:         aws.String(key),
		Body:        f,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

// DeleteFromS3 removes an object from the bucket. Safe to call with an empty
// key or when S3 is disabled.
func DeleteFromS3(ctx context.Context, key string) error {
	if engine == nil || key == "" {
		return nil
	}
	_, err := engine.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(engine.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		logger.Warn("FileCore", "Failed to delete S3 object", "key", key, "error", err)
	}
	return err
}

// GetObject opens a streaming download from S3 for a key, optionally applying
// a byte-range ("bytes=0-1048576"). The returned body MUST be closed by the
// caller. Returns (nil, nil) when S3 is disabled so callers can fall back.
func GetObject(ctx context.Context, key, rangeHeader string) (*s3.GetObjectOutput, error) {
	if engine == nil {
		return nil, nil
	}
	if key == "" {
		return nil, errors.New("empty object key")
	}
	in := &s3.GetObjectInput{
		Bucket: aws.String(engine.BucketName),
		Key:    aws.String(key),
	}
	if rangeHeader != "" {
		in.Range = aws.String(rangeHeader)
	}
	return engine.S3Client.GetObject(ctx, in)
}

// HeadObject returns object metadata (size, content-type, …) without
// downloading the body. Returns (nil, nil) when S3 is disabled.
func HeadObject(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	if engine == nil {
		return nil, nil
	}
	if key == "" {
		return nil, errors.New("empty object key")
	}
	return engine.S3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(engine.BucketName),
		Key:    aws.String(key),
	})
}
