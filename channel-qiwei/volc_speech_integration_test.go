package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	runLiveSpeechTestEnv = "QIWEI_RUN_LIVE_SPEECH_TEST"
	publicSpeechSample   = "https://raw.githubusercontent.com/microsoft/cognitive-services-speech-sdk/master/samples/csharp/sharedcontent/whatstheweatherlike.wav"
)

func TestVolcSpeechRecognizerLive(t *testing.T) {
	if os.Getenv(runLiveSpeechTestEnv) != "1" {
		t.Skip("set QIWEI_RUN_LIVE_SPEECH_TEST=1 to run live speech integration test")
	}

	cfg := LoadConfig()
	if strings.TrimSpace(cfg.VolcSpeechAppKey) == "" ||
		strings.TrimSpace(cfg.VolcSpeechAccessKey) == "" ||
		strings.TrimSpace(cfg.VolcSpeechResourceID) == "" {
		t.Skip("speech config is incomplete")
	}

	recognizer := newVolcengineRecognizer(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	taskID, err := recognizer.SubmitAudioTranscription(ctx, parsedAttachment{
		Kind:      "audio",
		Name:      "whatstheweatherlike.wav",
		SourceURL: publicSpeechSample,
	})
	if err != nil {
		t.Fatalf("submit audio transcription failed: %v", err)
	}
	if strings.TrimSpace(taskID) == "" {
		t.Fatal("submit audio transcription returned empty task id")
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		parsed, done, err := recognizer.QueryAudioTranscription(ctx, taskID)
		if err != nil {
			t.Fatalf("query audio transcription failed: %v", err)
		}
		if done {
			text := strings.TrimSpace(parsed.ParsedText)
			if text == "" {
				t.Fatal("transcription completed but returned empty text")
			}
			t.Logf("live transcript: %s", text)
			return
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatal("audio transcription did not finish within timeout")
}
