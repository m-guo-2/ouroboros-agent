package oss

import (
	"context"
	"io"
	"time"
)

// Storage defines the public upload/download contract for shared OSS access.
type Storage interface {
	PutObject(ctx context.Context, input PutObjectInput) (PutObjectResult, error)
	GetObject(ctx context.Context, key string) (*GetObjectResult, error)
	PresignGetURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}

type PutObjectInput struct {
	Key         string
	FileName    string
	ContentType string
	Size        int64
	Body        io.Reader
	Metadata    map[string]string
}

type PutObjectResult struct {
	Bucket      string
	Key         string
	ETag        string
	VersionID   string
	Size        int64
	ContentType string
}

type GetObjectResult struct {
	Bucket      string
	Key         string
	ETag        string
	VersionID   string
	Size        int64
	ContentType string
	Body        io.ReadCloser
}
