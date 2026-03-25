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

// Init lazily initializes the global renderer with OSS storage from environment
// config. Safe to call multiple times; only the first call takes effect.
func Init() error {
	initOnce.Do(func() {
		cfg := oss.LoadConfigFromEnv()
		if err := cfg.Validate(); err != nil {
			initErr = fmt.Errorf("cardrender: OSS config invalid: %w", err)
			return
		}
		storage, err := oss.NewMinIOStorage(cfg)
		if err != nil {
			initErr = fmt.Errorf("cardrender: failed to create OSS storage: %w", err)
			return
		}
		normalizedCfg, _ := cfg.Normalized()
		globalOSSCfg = normalizedCfg
		globalStorage = storage
		globalRenderer = NewRenderer(storage)
	})
	return initErr
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
