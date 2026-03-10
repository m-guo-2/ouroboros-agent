package oss

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTimeout = 30 * time.Second
)

// Config holds the runtime configuration for the shared OSS client.
type Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	Prefix    string
	UseSSL    bool
	Timeout   time.Duration
}

// LoadConfigFromEnv reads the shared OSS settings from process environment.
func LoadConfigFromEnv() Config {
	timeout := parseTimeout(
		strings.TrimSpace(os.Getenv("OSS_TIMEOUT")),
		strings.TrimSpace(os.Getenv("OSS_TIMEOUT_SECONDS")),
	)

	return Config{
		Endpoint:  strings.TrimSpace(os.Getenv("OSS_ENDPOINT")),
		Bucket:    strings.TrimSpace(os.Getenv("OSS_BUCKET")),
		AccessKey: strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY")),
		SecretKey: strings.TrimSpace(os.Getenv("OSS_SECRET_KEY")),
		Region:    strings.TrimSpace(os.Getenv("OSS_REGION")),
		Prefix:    strings.TrimSpace(os.Getenv("OSS_PREFIX")),
		UseSSL:    parseBool(os.Getenv("OSS_USE_SSL")),
		Timeout:   timeout,
	}
}

// Normalized returns a validated copy of the config.
func (c Config) Normalized() (Config, error) {
	cfg := c
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.AccessKey = strings.TrimSpace(cfg.AccessKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.Prefix = normalizeKeyPrefix(cfg.Prefix)

	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Timeout < 0 {
		return Config{}, wrapOperationError("validate_config", "", ErrConfig, fmt.Errorf("timeout must be >= 0"))
	}

	var err error
	cfg.Endpoint, cfg.UseSSL, err = normalizeEndpoint(cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		return Config{}, wrapOperationError("validate_config", "", ErrConfig, err)
	}

	switch {
	case cfg.Endpoint == "":
		return Config{}, wrapOperationError("validate_config", "", ErrConfig, fmt.Errorf("endpoint is required"))
	case cfg.Bucket == "":
		return Config{}, wrapOperationError("validate_config", "", ErrConfig, fmt.Errorf("bucket is required"))
	case cfg.AccessKey == "":
		return Config{}, wrapOperationError("validate_config", "", ErrConfig, fmt.Errorf("access key is required"))
	case cfg.SecretKey == "":
		return Config{}, wrapOperationError("validate_config", "", ErrConfig, fmt.Errorf("secret key is required"))
	}

	return cfg, nil
}

func (c Config) Validate() error {
	_, err := c.Normalized()
	return err
}

func normalizeEndpoint(endpoint string, useSSL bool) (string, bool, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", useSSL, nil
	}
	if !strings.Contains(endpoint, "://") {
		return strings.TrimRight(endpoint, "/"), useSSL, nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("invalid endpoint: %w", err)
	}
	if parsed.Host == "" {
		return "", false, fmt.Errorf("invalid endpoint: missing host")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false, fmt.Errorf("invalid endpoint: path is not allowed")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("invalid endpoint: query and fragment are not allowed")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		useSSL = false
	case "https":
		useSSL = true
	default:
		return "", false, fmt.Errorf("invalid endpoint scheme %q", parsed.Scheme)
	}
	return strings.TrimRight(parsed.Host, "/"), useSSL, nil
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseTimeout(durationValue, secondsValue string) time.Duration {
	if durationValue != "" {
		if d, err := time.ParseDuration(durationValue); err == nil {
			return d
		}
	}
	if secondsValue != "" {
		if n, err := strconv.Atoi(secondsValue); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return DefaultTimeout
}
