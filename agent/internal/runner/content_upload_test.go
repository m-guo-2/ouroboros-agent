package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m-guo-2/ouroboros-agent/shared/oss"
)

func TestLooksLikeFilePath(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"absolute path", "/tmp/image.png", true},
		{"relative path with slash", "data/output/chart.png", true},
		{"dot-relative path", "./result.pdf", true},
		{"plain text", "hello world", false},
		{"multi-word text", "this is a normal message", false},
		{"empty string", "", false},
		{"http url", "http://example.com/image.png", false},
		{"https url", "https://cdn.example.com/file.pdf", false},
		{"oss uri", "oss://bucket/key.png", false},
		{"multiline text", "line one\nline two", false},
		{"single word no slash", "hello", false},
		{"path with one space", "/tmp/my file.png", true},
		{"text with multiple spaces", "please send the file now", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeFilePath(tt.content); got != tt.want {
				t.Errorf("looksLikeFilePath(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestInferMessageTypeFromFile(t *testing.T) {
	dir := t.TempDir()

	mkFile := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	mkDir := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}

	pngFile := mkFile("chart.png")
	jpgFile := mkFile("photo.jpg")
	jpegFile := mkFile("photo.jpeg")
	gifFile := mkFile("anim.gif")
	webpFile := mkFile("hero.webp")
	bmpFile := mkFile("legacy.bmp")
	pdfFile := mkFile("report.pdf")
	txtFile := mkFile("notes.txt")
	noExtFile := mkFile("Makefile")
	subDir := mkDir("subdir")

	tests := []struct {
		name string
		path string
		want string
	}{
		{"png image", pngFile, "image"},
		{"jpg image", jpgFile, "image"},
		{"jpeg image", jpegFile, "image"},
		{"gif image", gifFile, "image"},
		{"webp image", webpFile, "image"},
		{"bmp image", bmpFile, "image"},
		{"pdf file", pdfFile, "file"},
		{"txt file", txtFile, "file"},
		{"no extension", noExtFile, "file"},
		{"directory", subDir, ""},
		{"nonexistent", filepath.Join(dir, "ghost.png"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferMessageTypeFromFile(tt.path); got != tt.want {
				t.Errorf("inferMessageTypeFromFile(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestAutoDetectIntegration(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "output.png")
	if err := os.WriteFile(imgPath, []byte("fakepng"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("empty messageType with image file path triggers detection", func(t *testing.T) {
		messageType := ""
		content := imgPath
		if !isMediaMessageType(messageType) && looksLikeFilePath(content) {
			if inferred := inferMessageTypeFromFile(content); inferred != "" {
				messageType = inferred
			}
		}
		if messageType != "image" {
			t.Errorf("messageType = %q, want %q", messageType, "image")
		}
	})

	t.Run("text messageType with image file path triggers detection", func(t *testing.T) {
		messageType := "text"
		content := imgPath
		if !isMediaMessageType(messageType) && looksLikeFilePath(content) {
			if inferred := inferMessageTypeFromFile(content); inferred != "" {
				messageType = inferred
			}
		}
		if messageType != "image" {
			t.Errorf("messageType = %q, want %q", messageType, "image")
		}
	})

	t.Run("explicit image messageType skips detection", func(t *testing.T) {
		messageType := "image"
		content := imgPath
		if !isMediaMessageType(messageType) && looksLikeFilePath(content) {
			if inferred := inferMessageTypeFromFile(content); inferred != "" {
				messageType = inferred
			}
		}
		if messageType != "image" {
			t.Errorf("messageType = %q, want %q", messageType, "image")
		}
	})

	t.Run("plain text content does not trigger detection", func(t *testing.T) {
		messageType := ""
		content := "hello, this is a normal reply"
		if !isMediaMessageType(messageType) && looksLikeFilePath(content) {
			if inferred := inferMessageTypeFromFile(content); inferred != "" {
				messageType = inferred
			}
		}
		if messageType != "" {
			t.Errorf("messageType = %q, want %q", messageType, "")
		}
	})

	t.Run("nonexistent file path does not change messageType", func(t *testing.T) {
		messageType := ""
		content := "/tmp/nonexistent_image_12345.png"
		if !isMediaMessageType(messageType) && looksLikeFilePath(content) {
			if inferred := inferMessageTypeFromFile(content); inferred != "" {
				messageType = inferred
			}
		}
		if messageType != "" {
			t.Errorf("messageType = %q, want %q", messageType, "")
		}
	})
}

func TestIsOSSURLForEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		rawURL   string
		want     bool
	}{
		{"matching endpoint with port", "minio:9000", "https://minio:9000/bucket/key?X-Amz-Algorithm=AWS4", true},
		{"matching endpoint no port", "oss.example.com", "https://oss.example.com/bucket/key?sig=abc", true},
		{"different host", "minio:9000", "https://other:9000/bucket/key", false},
		{"different port", "minio:9000", "https://minio:9001/bucket/key", false},
		{"empty endpoint", "", "https://minio:9000/bucket/key", false},
		{"invalid URL", "minio:9000", "://broken", false},
		{"public URL", "minio:9000", "http://115.190.14.209:2012/weixin/image.png", false},
		{"http scheme", "minio:9000", "http://minio:9000/bucket/key?token=xyz", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOSSURLForEndpoint(tt.endpoint, tt.rawURL); got != tt.want {
				t.Errorf("isOSSURLForEndpoint(%q, %q) = %v, want %v", tt.endpoint, tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestReuploadHTTPMedia(t *testing.T) {
	imgData := []byte("fake-png-data-for-testing")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/images/chart.png" {
			w.Header().Set("Content-Type", "image/png")
			w.Write(imgData)
			return
		}
		if r.URL.Path == "/gone" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	store := oss.NewFakeStorage()
	store.PresignBaseURL = "https://fake-oss.local"

	t.Run("downloads and uploads to OSS", func(t *testing.T) {
		result, err := reuploadHTTPMedia(context.Background(), store, srv.URL+"/images/chart.png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(result, "https://fake-oss.local/") {
			t.Errorf("result URL = %q, want prefix https://fake-oss.local/", result)
		}
		if !strings.Contains(result, "chart") {
			t.Errorf("result URL = %q, expected to contain filename stem", result)
		}
		if len(store.Objects) == 0 {
			t.Fatal("expected at least one object in fake storage")
		}
		for _, obj := range store.Objects {
			if string(obj.Body) != string(imgData) {
				t.Errorf("stored body = %q, want %q", string(obj.Body), string(imgData))
			}
			if obj.ContentType != "image/png" {
				t.Errorf("stored content type = %q, want image/png", obj.ContentType)
			}
		}
	})

	t.Run("returns error on HTTP failure", func(t *testing.T) {
		_, err := reuploadHTTPMedia(context.Background(), store, srv.URL+"/gone")
		if err == nil {
			t.Fatal("expected error for 404 response")
		}
		if !strings.Contains(err.Error(), "HTTP 404") {
			t.Errorf("error = %q, want to contain 'HTTP 404'", err.Error())
		}
	})

	t.Run("returns error on unreachable host", func(t *testing.T) {
		_, err := reuploadHTTPMedia(context.Background(), store, "http://192.0.2.1:1/unreachable.png")
		if err == nil {
			t.Fatal("expected error for unreachable host")
		}
	})

	t.Run("returns error on OSS upload failure", func(t *testing.T) {
		failStore := oss.NewFakeStorage()
		failStore.PutErr = oss.ErrInternal
		_, err := reuploadHTTPMedia(context.Background(), failStore, srv.URL+"/images/chart.png")
		if err == nil {
			t.Fatal("expected error on OSS put failure")
		}
		if !strings.Contains(err.Error(), "OSS") {
			t.Errorf("error = %q, want to mention OSS", err.Error())
		}
	})
}
