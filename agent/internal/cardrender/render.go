package cardrender

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/m-guo-2/ouroboros-agent/shared/oss"
)

const (
	defaultWidth    = 600
	defaultTimeout  = 30 * time.Second
	imageExpiry     = 7 * 24 * time.Hour
	ossKeyPrefix    = "card-renders"
	ossContentType  = "image/png"
)

// RenderOptions configures the rendering behavior.
type RenderOptions struct {
	Width   int
	Timeout time.Duration
}

// RenderResult holds the output of a successful render.
type RenderResult struct {
	ImageURL string
	OSSKey   string
	Width    int
	Height   int
}

// Renderer performs HTML → PNG → OSS upload.
type Renderer struct {
	storage oss.Storage
}

// NewRenderer creates a Renderer that uploads images to the given storage.
func NewRenderer(storage oss.Storage) *Renderer {
	return &Renderer{storage: storage}
}

// RenderCard takes an HTML string, renders it to PNG via headless Chrome,
// uploads the PNG to OSS, and returns the presigned URL.
func (r *Renderer) RenderCard(ctx context.Context, html string, opts RenderOptions) (*RenderResult, error) {
	html = strings.TrimSpace(html)
	if html == "" {
		return nil, newRenderError(ErrInvalidInput, "html content is empty", nil)
	}

	width := opts.Width
	if width <= 0 {
		width = defaultWidth
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	renderCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pngData, imgWidth, imgHeight, err := r.screenshot(renderCtx, html, width)
	if err != nil {
		return nil, err
	}

	if len(pngData) == 0 || imgWidth == 0 || imgHeight == 0 {
		return nil, newRenderError(ErrScreenshotEmpty, "screenshot produced empty or zero-size image", nil)
	}

	ossKey := fmt.Sprintf("%s/%d.png", ossKeyPrefix, time.Now().UnixNano())
	putResult, err := r.storage.PutObject(ctx, oss.PutObjectInput{
		Key:         ossKey,
		ContentType: ossContentType,
		Size:        int64(len(pngData)),
		Body:        bytes.NewReader(pngData),
	})
	if err != nil {
		return nil, newRenderError(ErrOSSUploadFailed, "failed to upload screenshot to OSS", err)
	}

	imageURL, err := r.storage.PresignGetURL(ctx, putResult.Key, imageExpiry)
	if err != nil {
		return nil, newRenderError(ErrOSSUploadFailed, "failed to generate presigned URL", err)
	}

	return &RenderResult{
		ImageURL: imageURL,
		OSSKey:   putResult.Key,
		Width:    imgWidth,
		Height:   imgHeight,
	}, nil
}

func (r *Renderer) screenshot(ctx context.Context, html string, viewportWidth int) ([]byte, int, int, error) {
	browser, err := pool.acquire(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	defer pool.release()

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, 0, 0, newRenderError(ErrBrowserUnavailable, "failed to create page", err)
	}
	defer page.Close()

	page = page.Context(ctx)

	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  viewportWidth,
		Height: 800,
		Mobile: false,
	}); err != nil {
		return nil, 0, 0, newRenderError(ErrBrowserUnavailable, "failed to set viewport", err)
	}

	if err := page.SetDocumentContent(html); err != nil {
		return nil, 0, 0, newRenderError(ErrBrowserUnavailable, "failed to set page content", err)
	}

	if err := page.WaitStable(500 * time.Millisecond); err != nil {
		if ctx.Err() != nil {
			return nil, 0, 0, newRenderError(ErrRenderTimeout, "page did not stabilize within timeout", err)
		}
	}

	metrics, err := page.Eval(`() => {
		const body = document.body;
		const html = document.documentElement;
		return {
			width: Math.max(body.scrollWidth, html.scrollWidth, body.offsetWidth, html.offsetWidth),
			height: Math.max(body.scrollHeight, html.scrollHeight, body.offsetHeight, html.offsetHeight)
		};
	}`)
	if err != nil {
		return nil, 0, 0, newRenderError(ErrBrowserUnavailable, "failed to get page dimensions", err)
	}

	pageWidth := metrics.Value.Get("width").Int()
	pageHeight := metrics.Value.Get("height").Int()
	if pageWidth == 0 || pageHeight == 0 {
		return nil, 0, 0, newRenderError(ErrScreenshotEmpty, "page has zero dimensions", nil)
	}

	img, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
		Clip: &proto.PageViewport{
			X:      0,
			Y:      0,
			Width:  float64(pageWidth),
			Height: float64(pageHeight),
			Scale:  2,
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, 0, newRenderError(ErrRenderTimeout, "screenshot timed out", err)
		}
		return nil, 0, 0, newRenderError(ErrBrowserUnavailable, "screenshot failed", err)
	}

	return img, pageWidth, pageHeight, nil
}
