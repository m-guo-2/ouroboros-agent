package cardrender

import (
	"fmt"
	"sync"

	"github.com/m-guo-2/ouroboros-agent/shared/oss"
)

var (
	globalRenderer *Renderer
	globalStorage  oss.Storage
	globalOSSCfg   oss.Config
	initOnce       sync.Once
	initErr        error
)

// InitWith initializes the global renderer with an externally provided OSS config.
// This is the preferred entry point — call it from main() after loading YAML config.
// Safe to call multiple times; only the first call takes effect.
func InitWith(cfg oss.Config) error {
	initOnce.Do(func() {
		initErr = doInit(cfg)
	})
	return initErr
}

// Init lazily initializes the global renderer with OSS storage from environment
// variables. Kept as fallback for backward compatibility; prefer InitWith.
func Init() error {
	initOnce.Do(func() {
		initErr = doInit(oss.LoadConfigFromEnv())
	})
	return initErr
}

func doInit(cfg oss.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("cardrender: OSS config invalid: %w", err)
	}
	storage, err := oss.NewMinIOStorage(cfg)
	if err != nil {
		return fmt.Errorf("cardrender: failed to create OSS storage: %w", err)
	}
	normalizedCfg, _ := cfg.Normalized()
	globalOSSCfg = normalizedCfg
	globalStorage = storage
	globalRenderer = NewRenderer(storage)
	return nil
}

// OSSStorage returns the shared OSS storage instance initialized by Init().
// Returns nil if Init() has not been called or failed.
func OSSStorage() oss.Storage {
	return globalStorage
}

// DefaultRenderer returns the globally initialized renderer.
// Returns nil if Init() has not been called or failed.
func DefaultRenderer() *Renderer {
	return globalRenderer
}

// OSSEndpoint returns the normalized OSS endpoint (host:port, no scheme).
// Returns "" if Init() has not been called or failed.
func OSSEndpoint() string {
	return globalOSSCfg.Endpoint
}

// Available reports whether the cardrender system is initialized and ready.
func Available() bool {
	return globalRenderer != nil
}
