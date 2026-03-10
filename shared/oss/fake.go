package oss

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type FakeObject struct {
	Body        []byte
	ContentType string
	ETag        string
	VersionID   string
}

// FakeStorage is an in-memory Storage implementation for unit tests and local callers.
type FakeStorage struct {
	mu             sync.Mutex
	Objects        map[string]FakeObject
	PutErr         error
	GetErr         error
	PresignErr     error
	PresignBaseURL string
}

func NewFakeStorage() *FakeStorage {
	return &FakeStorage{
		Objects: make(map[string]FakeObject),
	}
}

func (s *FakeStorage) PutObject(_ context.Context, input PutObjectInput) (PutObjectResult, error) {
	if s.PutErr != nil {
		return PutObjectResult{}, s.PutErr
	}
	if input.Body == nil {
		return PutObjectResult{}, wrapOperationError(opPut, input.Key, ErrConfig, fmt.Errorf("body is required"))
	}

	key := normalizeObjectKey("", input.Key)
	if key == "" {
		var err error
		key, err = GenerateObjectKey("", input.FileName)
		if err != nil {
			return PutObjectResult{}, err
		}
	}

	body, err := io.ReadAll(input.Body)
	if err != nil {
		return PutObjectResult{}, wrapOperationError(opPut, key, ErrInternal, err)
	}

	contentType := input.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.Objects[key] = FakeObject{
		Body:        append([]byte(nil), body...),
		ContentType: contentType,
	}
	return PutObjectResult{
		Key:         key,
		Size:        int64(len(body)),
		ContentType: contentType,
	}, nil
}

func (s *FakeStorage) GetObject(_ context.Context, key string) (*GetObjectResult, error) {
	if s.GetErr != nil {
		return nil, s.GetErr
	}

	key = normalizeObjectKey("", key)
	s.mu.Lock()
	obj, ok := s.Objects[key]
	s.mu.Unlock()
	if !ok {
		return nil, wrapOperationError(opGet, key, ErrNotFound, fmt.Errorf("object not found"))
	}

	contentType := obj.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &GetObjectResult{
		Key:         key,
		ETag:        obj.ETag,
		VersionID:   obj.VersionID,
		Size:        int64(len(obj.Body)),
		ContentType: contentType,
		Body:        io.NopCloser(bytes.NewReader(obj.Body)),
	}, nil
}

func (s *FakeStorage) PresignGetURL(_ context.Context, key string, _ time.Duration) (string, error) {
	if s.PresignErr != nil {
		return "", s.PresignErr
	}
	key = normalizeObjectKey("", key)
	if key == "" {
		return "", wrapOperationError(opSign, "", ErrConfig, fmt.Errorf("object key is required"))
	}
	base := strings.TrimRight(strings.TrimSpace(s.PresignBaseURL), "/")
	if base == "" {
		base = "https://fake-oss.local"
	}
	return base + "/" + key, nil
}
