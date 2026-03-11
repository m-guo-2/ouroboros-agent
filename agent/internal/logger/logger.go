// Package logger is a thin facade over shared/logger for the agent service.
// All agent code continues to import "agent/internal/logger" — no import changes needed.
package logger

import (
	"context"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

type Level = sharedlogger.Level

const (
	LBoundary = sharedlogger.LBoundary
	LBusiness = sharedlogger.LBusiness
	LDetail   = sharedlogger.LDetail
)

func Init(dir string) {
	sharedlogger.Init(dir, "agent")
}

func WithTrace(ctx context.Context, traceID, sessionID string) context.Context {
	return sharedlogger.WithTrace(ctx, traceID, sessionID)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return sharedlogger.WithRequestID(ctx, requestID)
}

func GetTrace(ctx context.Context) (traceID, sessionID string) {
	return sharedlogger.GetTrace(ctx)
}

func GetRequestID(ctx context.Context) string {
	return sharedlogger.GetRequestID(ctx)
}

func Boundary(ctx context.Context, msg string, args ...any) {
	sharedlogger.Boundary(ctx, msg, args...)
}

func Business(ctx context.Context, msg string, args ...any) {
	sharedlogger.Business(ctx, msg, args...)
}

func Detail(ctx context.Context, msg string, args ...any) {
	sharedlogger.Detail(ctx, msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	sharedlogger.Error(ctx, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	sharedlogger.Warn(ctx, msg, args...)
}

func WriteLLMIO(ctx context.Context, iteration int, request, response any) string {
	return sharedlogger.WriteLLMIO(ctx, iteration, request, response)
}

func ReadLLMIO(ref string) ([]byte, error) {
	return sharedlogger.ReadLLMIO(ref)
}

func Flush() {
	sharedlogger.Flush()
}
