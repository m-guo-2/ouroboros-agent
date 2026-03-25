package runner

import (
	"os"
	"path/filepath"
	"testing"
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
