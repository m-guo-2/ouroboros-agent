package logger

import (
	"bytes"
	"io"
	"net/http"
	"time"
)

type tracedTransport struct {
	base    http.RoundTripper
	service string
}

// NewTransport wraps a base RoundTripper with boundary-level logging for every
// outbound HTTP request and response. service identifies the external dependency
// (e.g. "anthropic", "qiwei-api", "volcengine").
func NewTransport(service string, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &tracedTransport{base: base, service: service}
}

// NewClient creates an *http.Client with traced transport and the given timeout.
func NewClient(service string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewTransport(service, nil),
	}
}

func (t *tracedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	ctx := req.Context()

	traceID, _ := GetTrace(ctx)
	requestID := GetRequestID(ctx)
	if traceID != "" {
		req.Header.Set("X-Trace-Id", traceID)
	}
	if requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}

	var reqBody []byte
	if req.Body != nil {
		reqBody, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	resp, err := t.base.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		Boundary(ctx, "外部调用失败",
			"direction", "outbound",
			"service", t.service,
			"method", req.Method,
			"url", req.URL.String(),
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
			"requestBody", string(reqBody),
			"requestSize", len(reqBody),
		)
		return nil, err
	}

	respBody, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	Boundary(ctx, "外部调用",
		"direction", "outbound",
		"service", t.service,
		"method", req.Method,
		"url", req.URL.String(),
		"status", resp.StatusCode,
		"duration_ms", duration.Milliseconds(),
		"requestBody", string(reqBody),
		"requestSize", len(reqBody),
		"responseBody", string(respBody),
		"responseSize", len(respBody),
	)

	return resp, nil
}
