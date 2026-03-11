package logger

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

type MiddlewareOptions struct {
	SkipPaths map[string]bool
}

func Middleware(opts MiddlewareOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if opts.SkipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			requestID := r.Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = GenerateRequestID()
			}

			var reqBody []byte
			if r.Body != nil {
				reqBody, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(reqBody))
			}

			ctx := WithRequestID(r.Context(), requestID)
			r = r.WithContext(ctx)

			Boundary(ctx, "HTTP 请求",
				"requestId", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"remoteAddr", r.RemoteAddr,
				"body", string(reqBody),
				"bodySize", len(reqBody),
			)

			rec := &responseRecorder{ResponseWriter: w, statusCode: 200}
			w.Header().Set("X-Request-Id", requestID)

			next.ServeHTTP(rec, r)

			duration := time.Since(start)
			Boundary(ctx, "HTTP 响应",
				"requestId", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.statusCode,
				"duration_ms", duration.Milliseconds(),
				"responseBody", rec.body.String(),
				"responseSize", rec.body.Len(),
			)
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ContextFromRequest extracts logger context from an incoming request,
// reading X-Request-Id and X-Trace-Id headers.
func ContextFromRequest(ctx context.Context, r *http.Request) context.Context {
	if rid := r.Header.Get("X-Request-Id"); rid != "" {
		ctx = WithRequestID(ctx, rid)
	}
	if tid := r.Header.Get("X-Trace-Id"); tid != "" {
		ctx = WithTrace(ctx, tid, "")
	}
	return ctx
}
