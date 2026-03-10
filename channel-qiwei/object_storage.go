package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	sharedoss "github.com/m-guo-2/ouroboros-agent/shared/oss"
)

const ossURIScheme = "oss://"

type objectStorageRuntime struct {
	store sharedoss.Storage
	cfg   sharedoss.Config
}

func newObjectStorage(logger *slog.Logger) objectStorageRuntime {
	cfg := sharedoss.LoadConfigFromEnv()
	if !hasAnyOSSConfig(cfg) {
		return objectStorageRuntime{}
	}

	normalized, err := cfg.Normalized()
	if err != nil {
		logger.Warn("oss storage disabled", "err", err)
		return objectStorageRuntime{}
	}

	store, err := sharedoss.NewMinIOStorage(normalized)
	if err != nil {
		logger.Warn("oss storage init failed", "err", err)
		return objectStorageRuntime{}
	}
	return objectStorageRuntime{
		store: store,
		cfg:   normalized,
	}
}

func hasAnyOSSConfig(cfg sharedoss.Config) bool {
	return strings.TrimSpace(cfg.Endpoint) != "" ||
		strings.TrimSpace(cfg.Bucket) != "" ||
		strings.TrimSpace(cfg.AccessKey) != "" ||
		strings.TrimSpace(cfg.SecretKey) != "" ||
		strings.TrimSpace(cfg.Region) != "" ||
		strings.TrimSpace(cfg.Prefix) != ""
}

func formatObjectURI(bucket, key string) string {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if bucket == "" {
		return key
	}
	if key == "" {
		return ossURIScheme + bucket
	}
	return ossURIScheme + bucket + "/" + key
}

func publicObjectURL(cfg Config, storageCfg sharedoss.Config, bucket, key string) string {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return ""
	}

	if base := strings.TrimSpace(cfg.OSSPublicBaseURL); base != "" {
		return strings.TrimRight(base, "/") + "/" + key
	}

	endpoint := strings.TrimSpace(storageCfg.Endpoint)
	bucket = strings.TrimSpace(bucket)
	if endpoint == "" || bucket == "" {
		return formatObjectURI(bucket, key)
	}

	scheme := "http"
	if storageCfg.UseSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, endpoint, bucket, key)
}

func (a *app) accessibleObjectURL(ctx context.Context, bucket, key string) string {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return ""
	}
	if base := strings.TrimSpace(a.cfg.OSSPublicBaseURL); base != "" {
		return publicObjectURL(a.cfg, a.storageConfig, bucket, key)
	}
	if a.storage != nil {
		if signed, err := a.storage.PresignGetURL(ctx, key, 15*time.Minute); err == nil && strings.TrimSpace(signed) != "" {
			return signed
		}
	}
	return publicObjectURL(a.cfg, a.storageConfig, bucket, key)
}

func parseObjectURI(raw string) (bucket, key string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("resource uri is required")
	}
	if !strings.HasPrefix(raw, ossURIScheme) {
		return "", "", fmt.Errorf("unsupported resource uri: %s", raw)
	}
	trimmed := strings.TrimPrefix(raw, ossURIScheme)
	parts := strings.SplitN(trimmed, "/", 2)
	bucket = strings.TrimSpace(parts[0])
	if bucket == "" {
		return "", "", fmt.Errorf("oss resource uri missing bucket")
	}
	if len(parts) == 2 {
		key = strings.TrimSpace(parts[1])
	}
	if key == "" {
		return "", "", fmt.Errorf("oss resource uri missing object key")
	}
	return bucket, key, nil
}

func resourceBaseName(resourceURI string) string {
	resourceURI = strings.TrimSpace(resourceURI)
	switch {
	case strings.HasPrefix(resourceURI, ossURIScheme):
		_, key, err := parseObjectURI(resourceURI)
		if err == nil {
			return path.Base(key)
		}
	case strings.HasPrefix(resourceURI, "http://") || strings.HasPrefix(resourceURI, "https://"):
		if parsed, err := url.Parse(resourceURI); err == nil {
			if base := path.Base(parsed.Path); base != "" && base != "." && base != "/" {
				return base
			}
		}
	default:
		if base := filepath.Base(resourceURI); base != "" && base != "." && base != "/" {
			return base
		}
	}
	return "attachment"
}

func (a *app) uploadDownloadedAttachment(ctx context.Context, attachment parsedAttachment, body io.Reader, contentType string, size int64) (string, string, error) {
	if a.storage == nil {
		return "", "", fmt.Errorf("oss storage is not configured")
	}
	if contentType == "" {
		contentType = mimeTypeForResource(attachment.Name, attachment.Kind)
	}

	result, err := a.storage.PutObject(ctx, sharedoss.PutObjectInput{
		FileName:    attachment.Name,
		ContentType: contentType,
		Size:        size,
		Body:        body,
		Metadata: map[string]string{
			"source": "channel-qiwei",
			"kind":   attachment.Kind,
		},
	})
	if err != nil {
		return "", "", err
	}
	if result.ContentType != "" {
		contentType = result.ContentType
	}
	return a.accessibleObjectURL(ctx, result.Bucket, result.Key), contentType, nil
}

func (a *app) readPreparedResource(ctx context.Context, attachment parsedAttachment) ([]byte, string, error) {
	resourceURI := strings.TrimSpace(firstNonEmpty(attachment.ResourceURI, attachment.LocalPath))
	switch {
	case strings.HasPrefix(resourceURI, ossURIScheme):
		if a.storage == nil {
			return nil, "", fmt.Errorf("oss storage is not configured")
		}
		_, key, err := parseObjectURI(resourceURI)
		if err != nil {
			return nil, "", err
		}
		got, err := a.storage.GetObject(ctx, key)
		if err != nil {
			return nil, "", err
		}
		defer got.Body.Close()
		raw, err := io.ReadAll(got.Body)
		if err != nil {
			return nil, "", err
		}
		return raw, got.ContentType, nil
	case strings.HasPrefix(resourceURI, "http://") || strings.HasPrefix(resourceURI, "https://"):
		resp, err := a.http.Get(resourceURI)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
		}
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", err
		}
		return raw, resp.Header.Get("Content-Type"), nil
	default:
		raw, err := os.ReadFile(resourceURI)
		if err != nil {
			return nil, "", err
		}
		return raw, mimeTypeForResource(resourceURI, attachment.Kind), nil
	}
}

func mimeTypeForResource(name, kind string) string {
	if detected := mimeTypeByName(name); detected != "" {
		return detected
	}
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".silk":
		return "audio/silk"
	case ".ogg":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	}
	switch kind {
	case "image":
		return "image/jpeg"
	case "audio":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func mimeTypeByName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(name))))
}
