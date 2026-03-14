package cardrender

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRenderCard_EmptyHTML(t *testing.T) {
	r := NewRenderer(nil)
	_, err := r.RenderCard(context.Background(), "", RenderOptions{})
	if err == nil {
		t.Fatal("expected error for empty HTML")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}
}

func TestRenderCard_WhitespaceHTML(t *testing.T) {
	r := NewRenderer(nil)
	_, err := r.RenderCard(context.Background(), "   \n  ", RenderOptions{})
	if err == nil {
		t.Fatal("expected error for whitespace-only HTML")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}
}

func TestRenderError_Unwrap(t *testing.T) {
	err := newRenderError(ErrBrowserUnavailable, "test", nil)
	if !errors.Is(err, ErrBrowserUnavailable) {
		t.Error("expected Unwrap to return category error")
	}
}

func TestRenderError_Message(t *testing.T) {
	cause := errors.New("root cause")
	err := newRenderError(ErrOSSUploadFailed, "upload failed", cause)
	msg := err.Error()
	if !strings.Contains(msg, "upload failed") {
		t.Errorf("expected message in error, got: %s", msg)
	}
	if !strings.Contains(msg, "root cause") {
		t.Errorf("expected cause in error, got: %s", msg)
	}
}

func TestPreprocessData_RankingMaxValue(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"name": "A", "value": float64(50)},
			map[string]interface{}{"name": "B", "value": float64(100)},
			map[string]interface{}{"name": "C", "value": float64(75)},
		},
	}

	result := preprocessData("ranking", data)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("expected map result")
	}

	maxVal, ok := m["maxValue"].(float64)
	if !ok {
		t.Fatal("expected maxValue to be float64")
	}
	if maxVal != 100 {
		t.Errorf("expected maxValue=100, got %f", maxVal)
	}
}

func TestPreprocessData_NonRanking(t *testing.T) {
	data := map[string]interface{}{"title": "test"}
	result := preprocessData("kpi", data)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("expected map result")
	}
	if _, exists := m["maxValue"]; exists {
		t.Error("maxValue should not be set for non-ranking templates")
	}
}
