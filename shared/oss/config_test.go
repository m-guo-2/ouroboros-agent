package oss

import (
	"errors"
	"testing"
	"time"
)

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("OSS_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("OSS_BUCKET", "media")
	t.Setenv("OSS_ACCESS_KEY", "ak")
	t.Setenv("OSS_SECRET_KEY", "sk")
	t.Setenv("OSS_REGION", "us-east-1")
	t.Setenv("OSS_PREFIX", "/team/uploads/")
	t.Setenv("OSS_USE_SSL", "true")
	t.Setenv("OSS_TIMEOUT_SECONDS", "42")

	cfg, err := LoadConfigFromEnv().Normalized()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv().Normalized() error = %v", err)
	}

	if cfg.Endpoint != "127.0.0.1:9000" {
		t.Fatalf("Endpoint = %q, want %q", cfg.Endpoint, "127.0.0.1:9000")
	}
	if cfg.UseSSL {
		t.Fatalf("UseSSL = true, want false because http:// endpoint should override it")
	}
	if cfg.Prefix != "team/uploads" {
		t.Fatalf("Prefix = %q, want %q", cfg.Prefix, "team/uploads")
	}
	if cfg.Timeout != 42*time.Second {
		t.Fatalf("Timeout = %v, want %v", cfg.Timeout, 42*time.Second)
	}
}

func TestConfigNormalizedRejectsMissingRequiredFields(t *testing.T) {
	_, err := (Config{}).Normalized()
	if err == nil {
		t.Fatal("Normalized() error = nil, want config error")
	}
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("Normalized() error = %v, want ErrConfig", err)
	}
}

func TestConfigNormalizedSupportsHTTPSURL(t *testing.T) {
	cfg, err := (Config{
		Endpoint:  "https://minio.example.com:9443",
		Bucket:    "bucket",
		AccessKey: "ak",
		SecretKey: "sk",
	}).Normalized()
	if err != nil {
		t.Fatalf("Normalized() error = %v", err)
	}
	if cfg.Endpoint != "minio.example.com:9443" {
		t.Fatalf("Endpoint = %q, want stripped host", cfg.Endpoint)
	}
	if !cfg.UseSSL {
		t.Fatal("UseSSL = false, want true")
	}
}
