package main

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	runLiveVisionTestEnv   = "QIWEI_RUN_LIVE_VISION_TEST"
	runLiveDocumentTestEnv = "QIWEI_RUN_LIVE_DOCUMENT_TEST"
)

func TestVolcVisionRecognizerLive(t *testing.T) {
	if os.Getenv(runLiveVisionTestEnv) != "1" {
		t.Skip("set QIWEI_RUN_LIVE_VISION_TEST=1 to run live vision integration test")
	}

	cfg := LoadConfig()
	if strings.TrimSpace(cfg.VolcArkAPIKey) == "" || strings.TrimSpace(cfg.VolcVisionModel) == "" {
		t.Skip("vision config is incomplete")
	}

	imagePath := writeVisionFixture(t)
	recognizer := newVolcengineRecognizer(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	parsed, err := recognizer.ParseImage(ctx, parsedAttachment{
		Kind:      "image",
		Name:      filepath.Base(imagePath),
		LocalPath: imagePath,
	})
	if err != nil {
		t.Fatalf("parse image failed: %v", err)
	}
	if strings.TrimSpace(parsed.ParsedText) == "" {
		t.Fatal("vision parsing returned empty text")
	}
	t.Logf("live vision result: %s", parsed.ParsedText)
}

func TestVolcDocumentRecognizerLive(t *testing.T) {
	if os.Getenv(runLiveDocumentTestEnv) != "1" {
		t.Skip("set QIWEI_RUN_LIVE_DOCUMENT_TEST=1 to run live document integration test")
	}

	cfg := LoadConfig()
	if strings.TrimSpace(cfg.VolcArkAPIKey) == "" || strings.TrimSpace(cfg.VolcDocumentModel) == "" {
		t.Skip("document config is incomplete")
	}

	docPath := writeDocumentFixture(t)
	recognizer := newVolcengineRecognizer(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	parsed, err := recognizer.ParseDocument(ctx, parsedAttachment{
		Kind:      "document",
		Name:      filepath.Base(docPath),
		LocalPath: docPath,
	})
	if err != nil {
		t.Fatalf("parse document failed: %v", err)
	}
	if strings.TrimSpace(parsed.ParsedText) == "" {
		t.Fatal("document parsing returned empty text")
	}
	t.Logf("live document result: %s", parsed.ParsedText)
}

func writeVisionFixture(t *testing.T) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "vision-sample.png")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create vision fixture: %v", err)
	}
	defer file.Close()

	img := image.NewRGBA(image.Rect(0, 0, 160, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 160; x++ {
			if x < 80 {
				img.Set(x, y, color.RGBA{R: 220, G: 50, B: 47, A: 255})
				continue
			}
			img.Set(x, y, color.RGBA{R: 38, G: 139, B: 210, A: 255})
		}
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode vision fixture: %v", err)
	}
	return filePath
}

func writeDocumentFixture(t *testing.T) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "document-sample.md")
	content := strings.Join([]string{
		"# 项目周报",
		"",
		"- 目标：验证文件理解链路是否能调通。",
		"- 进展：已经完成视觉模型和语音识别的基础接入测试。",
		"- 风险：二进制文档理解仍需后续扩展。",
		"",
		"请提炼本周的关键进展和风险。",
	}, "\n")
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write document fixture: %v", err)
	}
	return filePath
}
