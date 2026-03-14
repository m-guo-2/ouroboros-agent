package cardrender

import (
	"context"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

const (
	defaultIdleTimeout = 5 * time.Minute
	defaultMaxConcur   = 3
)

type browserPool struct {
	mu       sync.Mutex
	browser  *rod.Browser
	lastUsed time.Time
	sem      chan struct{}
	closed   bool
	stopIdle chan struct{}
}

var pool = &browserPool{
	sem:      make(chan struct{}, defaultMaxConcur),
	stopIdle: make(chan struct{}),
}

func (p *browserPool) acquire(ctx context.Context) (*rod.Browser, error) {
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, newRenderError(ErrRenderTimeout, "waiting for browser slot timed out", ctx.Err())
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		<-p.sem
		return nil, newRenderError(ErrBrowserUnavailable, "browser pool is closed", nil)
	}

	if p.browser != nil {
		p.lastUsed = time.Now()
		return p.browser, nil
	}

	b, err := p.launch()
	if err != nil {
		<-p.sem
		return nil, err
	}
	p.browser = b
	p.lastUsed = time.Now()
	go p.idleReaper()
	return b, nil
}

func (p *browserPool) release() {
	<-p.sem
}

func (p *browserPool) launch() (*rod.Browser, error) {
	l := launcher.New().
		Headless(true).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage")

	if path, found := launcher.LookPath(); found {
		l = l.Bin(path)
	}

	u, err := l.Launch()
	if err != nil {
		return nil, newRenderError(ErrBrowserUnavailable, "failed to launch chromium", err)
	}

	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		return nil, newRenderError(ErrBrowserUnavailable, "failed to connect to chromium", err)
	}
	return b, nil
}

func (p *browserPool) idleReaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			if p.browser != nil && time.Since(p.lastUsed) > defaultIdleTimeout {
				_ = p.browser.Close()
				p.browser = nil
				p.mu.Unlock()
				return
			}
			p.mu.Unlock()
		case <-p.stopIdle:
			return
		}
	}
}

// Close shuts down the browser pool and releases resources.
func Close() {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.closed = true
	if pool.browser != nil {
		_ = pool.browser.Close()
		pool.browser = nil
	}
	select {
	case pool.stopIdle <- struct{}{}:
	default:
	}
}
