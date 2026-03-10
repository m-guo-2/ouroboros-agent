package oss

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	neturl "net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	opInit = "init_client"
	opPut  = "put_object"
	opGet  = "get_object"
	opSign = "presign_get_url"
)

type backend interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams neturl.Values) (*neturl.URL, error)
}

type objectReader interface {
	io.ReadCloser
	Stat() (minio.ObjectInfo, error)
}

type minioBackend struct {
	client *minio.Client
}

func (b *minioBackend) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return b.client.PutObject(ctx, bucketName, objectName, reader, objectSize, opts)
}

func (b *minioBackend) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error) {
	return b.client.GetObject(ctx, bucketName, objectName, opts)
}

func (b *minioBackend) PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams neturl.Values) (*neturl.URL, error) {
	return b.client.PresignedGetObject(ctx, bucketName, objectName, expiry, reqParams)
}

// MinIOStorage is a MinIO-backed implementation of Storage.
type MinIOStorage struct {
	cfg     Config
	backend backend
	now     func() time.Time
	random  io.Reader
}

func NewMinIOStorage(cfg Config) (*MinIOStorage, error) {
	cfg, err := cfg.Normalized()
	if err != nil {
		return nil, err
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, wrapOperationError(opInit, "", ErrConfig, err)
	}

	return newMinIOStorageWithBackend(cfg, &minioBackend{client: client}), nil
}

func newMinIOStorageWithBackend(cfg Config, backend backend) *MinIOStorage {
	return &MinIOStorage{
		cfg:     cfg,
		backend: backend,
		now:     func() time.Time { return time.Now().UTC() },
		random:  rand.Reader,
	}
}

func (s *MinIOStorage) PutObject(ctx context.Context, input PutObjectInput) (PutObjectResult, error) {
	if input.Body == nil {
		return PutObjectResult{}, wrapOperationError(opPut, input.Key, ErrConfig, fmt.Errorf("body is required"))
	}

	key := strings.TrimSpace(input.Key)
	if key == "" {
		generatedKey, err := generateObjectKey(s.cfg.Prefix, input.FileName, s.now(), s.random)
		if err != nil {
			return PutObjectResult{}, err
		}
		key = generatedKey
	} else {
		key = normalizeObjectKey(s.cfg.Prefix, key)
	}
	if key == "" {
		return PutObjectResult{}, wrapOperationError(opPut, "", ErrConfig, fmt.Errorf("object key is required"))
	}

	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	info, err := s.backend.PutObject(ctx, s.cfg.Bucket, key, input.Body, input.Size, minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: input.Metadata,
	})
	if err != nil {
		return PutObjectResult{}, classifyError(opPut, key, err)
	}

	return PutObjectResult{
		Bucket:      s.cfg.Bucket,
		Key:         key,
		ETag:        info.ETag,
		VersionID:   info.VersionID,
		Size:        info.Size,
		ContentType: contentType,
	}, nil
}

func (s *MinIOStorage) GetObject(ctx context.Context, key string) (*GetObjectResult, error) {
	key = normalizeObjectKey("", key)
	if key == "" {
		return nil, wrapOperationError(opGet, "", ErrConfig, fmt.Errorf("object key is required"))
	}

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	body, err := s.backend.GetObject(ctx, s.cfg.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, classifyError(opGet, key, err)
	}

	info, err := body.Stat()
	if err != nil {
		_ = body.Close()
		return nil, classifyError(opGet, key, err)
	}

	contentType := strings.TrimSpace(info.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return &GetObjectResult{
		Bucket:      s.cfg.Bucket,
		Key:         key,
		ETag:        info.ETag,
		VersionID:   info.VersionID,
		Size:        info.Size,
		ContentType: contentType,
		Body:        body,
	}, nil
}

func (s *MinIOStorage) PresignGetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	key = normalizeObjectKey("", key)
	if key == "" {
		return "", wrapOperationError(opSign, "", ErrConfig, fmt.Errorf("object key is required"))
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	u, err := s.backend.PresignedGetObject(ctx, s.cfg.Bucket, key, expiry, nil)
	if err != nil {
		return "", classifyError(opSign, key, err)
	}
	if u == nil || strings.TrimSpace(u.String()) == "" {
		return "", wrapOperationError(opSign, key, ErrInternal, fmt.Errorf("empty presigned url"))
	}
	return u.String(), nil
}

func (s *MinIOStorage) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.cfg.Timeout > 0 {
		if _, ok := ctx.Deadline(); !ok {
			return context.WithTimeout(ctx, s.cfg.Timeout)
		}
	}
	return ctx, func() {}
}

func normalizeObjectKey(prefix, key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "\\", "/")
	key = strings.TrimLeft(key, "/")
	key = pathClean(key)
	if key == "." {
		key = ""
	}
	key = strings.Trim(key, "/")
	if key == "" {
		return normalizeKeyPrefix(prefix)
	}
	if normalizedPrefix := normalizeKeyPrefix(prefix); normalizedPrefix != "" {
		return normalizedPrefix + "/" + key
	}
	return key
}

func pathClean(key string) string {
	parts := strings.Split(key, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(clean) > 0 {
				clean = clean[:len(clean)-1]
			}
			continue
		}
		clean = append(clean, part)
	}
	return strings.Join(clean, "/")
}

func classifyError(op, key string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return wrapOperationError(op, key, ErrTransport, err)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return wrapOperationError(op, key, ErrTransport, err)
	}

	var urlErr *neturl.Error
	if errors.As(err, &urlErr) {
		return wrapOperationError(op, key, ErrTransport, err)
	}

	resp := minio.ToErrorResponse(err)
	switch {
	case resp.Code == "NoSuchKey" || resp.Code == "NoSuchBucket" || resp.StatusCode == 404:
		return wrapOperationError(op, key, ErrNotFound, err)
	case resp.Code == "AccessDenied" ||
		resp.Code == "InvalidAccessKeyId" ||
		resp.Code == "SignatureDoesNotMatch" ||
		resp.Code == "AuthorizationHeaderMalformed" ||
		resp.Code == "InvalidTokenId" ||
		resp.StatusCode == 401 ||
		resp.StatusCode == 403:
		return wrapOperationError(op, key, ErrAuthentication, err)
	case resp.Code == "RequestTimeout" || resp.Code == "SlowDown" || resp.StatusCode == 408 || resp.StatusCode == 504:
		return wrapOperationError(op, key, ErrTransport, err)
	default:
		return wrapOperationError(op, key, ErrInternal, err)
	}
}
