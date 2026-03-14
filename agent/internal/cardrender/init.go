package cardrender

import (
	"fmt"
	"sync"

	"github.com/m-guo-2/ouroboros-agent/shared/oss"
)

var (
	globalRenderer *Renderer
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
		globalRenderer = NewRenderer(storage)
	})
	return initErr
}

// DefaultRenderer returns the globally initialized renderer.
// Returns nil if Init() has not been called or failed.
func DefaultRenderer() *Renderer {
	return globalRenderer
}

// Available reports whether the cardrender system is initialized and ready.
func Available() bool {
	return globalRenderer != nil
}
