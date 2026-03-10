package oss

import (
	"bytes"
	"context"
	"errors"
	"io"
	neturl "net/url"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

type fakeBackend struct {
	putInfo           minio.UploadInfo
	putErr            error
	getObject         objectReader
	getErr            error
	presignedURL      *neturl.URL
	presignErr        error
	lastBucket        string
	lastKey           string
	lastContentType   string
	lastContentLength int64
	lastPayload       []byte
}

func (b *fakeBackend) PutObject(_ context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	b.lastBucket = bucketName
	b.lastKey = objectName
	b.lastContentType = opts.ContentType
	b.lastContentLength = objectSize
	b.lastPayload = payload
	if b.putErr != nil {
		return minio.UploadInfo{}, b.putErr
	}
	info := b.putInfo
	if info.Size == 0 {
		info.Size = int64(len(payload))
	}
	return info, nil
}

func (b *fakeBackend) GetObject(_ context.Context, bucketName, objectName string, _ minio.GetObjectOptions) (objectReader, error) {
	b.lastBucket = bucketName
	b.lastKey = objectName
	if b.getErr != nil {
		return nil, b.getErr
	}
	return b.getObject, nil
}

func (b *fakeBackend) PresignedGetObject(_ context.Context, bucketName, objectName string, _ time.Duration, _ neturl.Values) (*neturl.URL, error) {
	b.lastBucket = bucketName
	b.lastKey = objectName
	if b.presignErr != nil {
		return nil, b.presignErr
	}
	if b.presignedURL != nil {
		return b.presignedURL, nil
	}
	return neturl.Parse("https://signed.example.com/" + objectName)
}

type fakeObject struct {
	io.ReadCloser
	info    minio.ObjectInfo
	statErr error
}

func (o *fakeObject) Stat() (minio.ObjectInfo, error) {
	if o.statErr != nil {
		return minio.ObjectInfo{}, o.statErr
	}
	return o.info, nil
}

func TestPutObjectGeneratesKeyAndUploadsContent(t *testing.T) {
	cfg, err := (Config{
		Endpoint:  "127.0.0.1:9000",
		Bucket:    "bucket",
		AccessKey: "ak",
		SecretKey: "sk",
		Prefix:    "team/uploads",
		Timeout:   time.Second,
	}).Normalized()
	if err != nil {
		t.Fatalf("Normalized() error = %v", err)
	}

	backend := &fakeBackend{
		putInfo: minio.UploadInfo{ETag: "etag-1", VersionID: "v1"},
	}
	store := newMinIOStorageWithBackend(cfg, backend)
	store.now = func() time.Time {
		return time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	}
	store.random = bytes.NewReader([]byte{1, 2, 3, 4, 5, 6})

	result, err := store.PutObject(context.Background(), PutObjectInput{
		FileName:    "Avatar.PNG",
		ContentType: "image/png",
		Size:        4,
		Body:        bytes.NewReader([]byte("data")),
	})
	if err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}

	wantKey := "team/uploads/2026/03/10/avatar-010203040506.png"
	if result.Key != wantKey {
		t.Fatalf("PutObject().Key = %q, want %q", result.Key, wantKey)
	}
	if backend.lastBucket != "bucket" || backend.lastKey != wantKey {
		t.Fatalf("backend called with bucket/key = %q/%q", backend.lastBucket, backend.lastKey)
	}
	if backend.lastContentType != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", backend.lastContentType)
	}
}

func TestGetObjectReturnsMetadataAndBody(t *testing.T) {
	cfg, err := (Config{
		Endpoint:  "127.0.0.1:9000",
		Bucket:    "bucket",
		AccessKey: "ak",
		SecretKey: "sk",
	}).Normalized()
	if err != nil {
		t.Fatalf("Normalized() error = %v", err)
	}

	backend := &fakeBackend{
		getObject: &fakeObject{
			ReadCloser: io.NopCloser(bytes.NewReader([]byte("hello"))),
			info: minio.ObjectInfo{
				Key:         "docs/readme.txt",
				Size:        5,
				ETag:        "etag-1",
				VersionID:   "v1",
				ContentType: "text/plain",
			},
		},
	}
	store := newMinIOStorageWithBackend(cfg, backend)

	result, err := store.GetObject(context.Background(), "docs/readme.txt")
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	defer result.Body.Close()

	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("ReadAll(result.Body) error = %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", string(body))
	}
	if result.ContentType != "text/plain" {
		t.Fatalf("ContentType = %q, want text/plain", result.ContentType)
	}
}

func TestClassifyNotFoundError(t *testing.T) {
	cfg, err := (Config{
		Endpoint:  "127.0.0.1:9000",
		Bucket:    "bucket",
		AccessKey: "ak",
		SecretKey: "sk",
	}).Normalized()
	if err != nil {
		t.Fatalf("Normalized() error = %v", err)
	}
	store := newMinIOStorageWithBackend(cfg, &fakeBackend{
		getErr: minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404},
	})

	_, err = store.GetObject(context.Background(), "missing.txt")
	if err == nil {
		t.Fatal("GetObject() error = nil, want ErrNotFound")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetObject() error = %v, want ErrNotFound", err)
	}
}

func TestClassifyAuthenticationError(t *testing.T) {
	cfg, err := (Config{
		Endpoint:  "127.0.0.1:9000",
		Bucket:    "bucket",
		AccessKey: "ak",
		SecretKey: "sk",
	}).Normalized()
	if err != nil {
		t.Fatalf("Normalized() error = %v", err)
	}
	store := newMinIOStorageWithBackend(cfg, &fakeBackend{
		putErr: minio.ErrorResponse{Code: "AccessDenied", StatusCode: 403},
	})

	_, err = store.PutObject(context.Background(), PutObjectInput{
		Key:  "hello.txt",
		Body: bytes.NewReader([]byte("hello")),
		Size: 5,
	})
	if err == nil {
		t.Fatal("PutObject() error = nil, want ErrAuthentication")
	}
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("PutObject() error = %v, want ErrAuthentication", err)
	}
}

func TestFakeStorageRoundTrip(t *testing.T) {
	store := NewFakeStorage()

	putResult, err := store.PutObject(context.Background(), PutObjectInput{
		Key:         "notes/todo.txt",
		ContentType: "text/plain",
		Body:        bytes.NewReader([]byte("ship it")),
	})
	if err != nil {
		t.Fatalf("FakeStorage.PutObject() error = %v", err)
	}
	if putResult.Key != "notes/todo.txt" {
		t.Fatalf("PutObject().Key = %q, want notes/todo.txt", putResult.Key)
	}

	got, err := store.GetObject(context.Background(), putResult.Key)
	if err != nil {
		t.Fatalf("FakeStorage.GetObject() error = %v", err)
	}
	defer got.Body.Close()

	body, err := io.ReadAll(got.Body)
	if err != nil {
		t.Fatalf("ReadAll(got.Body) error = %v", err)
	}
	if string(body) != "ship it" {
		t.Fatalf("body = %q, want ship it", string(body))
	}
}

func TestPresignGetURLReturnsSignedURL(t *testing.T) {
	cfg, err := (Config{
		Endpoint:  "127.0.0.1:9000",
		Bucket:    "bucket",
		AccessKey: "ak",
		SecretKey: "sk",
	}).Normalized()
	if err != nil {
		t.Fatalf("Normalized() error = %v", err)
	}
	signed, _ := neturl.Parse("https://signed.example.com/audio/test.silk?token=abc")
	backend := &fakeBackend{presignedURL: signed}
	store := newMinIOStorageWithBackend(cfg, backend)

	got, err := store.PresignGetURL(context.Background(), "audio/test.silk", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetURL() error = %v", err)
	}
	if got != signed.String() {
		t.Fatalf("PresignGetURL() = %q, want %q", got, signed.String())
	}
}
