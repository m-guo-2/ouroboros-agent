package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

const runLiveSpeechTestEnv = "QIWEI_RUN_LIVE_SPEECH_TEST"

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

	wavData := generateTestWav(t)
	t.Logf("test wav: %d bytes", len(wavData))

	recognizer := newVolcengineRecognizer(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	taskID, err := recognizer.SubmitAudioTranscription(ctx, wavData, "wav")
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
			t.Logf("live transcript: %q", parsed.ParsedText)
			return
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatal("audio transcription did not finish within timeout")
}

func downloadPublicWav(t *testing.T) []byte {
	t.Helper()
	client := logger.NewClient("test", 30*time.Second)
	resp, err := client.Get("https://raw.githubusercontent.com/microsoft/cognitive-services-speech-sdk/master/samples/csharp/sharedcontent/whatstheweatherlike.wav")
	if err != nil {
		t.Fatalf("download sample wav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download sample wav: HTTP %d", resp.StatusCode)
	}
	var buf []byte
	buf, err = readAllBody(resp)
	if err != nil {
		t.Fatalf("read sample wav: %v", err)
	}
	return buf
}

func readAllBody(resp *http.Response) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}

func generateTestWav(t *testing.T) []byte {
	t.Helper()
	sr, dur, bits := 16000, 1, 16
	n := sr * dur
	dataSize := n * bits / 8
	totalSize := 44 + dataSize
	wav := make([]byte, totalSize)
	copy(wav[0:4], "RIFF")
	putLE32(wav, 4, uint32(totalSize-8))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	putLE32(wav, 16, 16)
	putLE16(wav, 20, 1)
	putLE16(wav, 22, 1)
	putLE32(wav, 24, uint32(sr))
	putLE32(wav, 28, uint32(sr*bits/8))
	putLE16(wav, 32, uint16(bits/8))
	putLE16(wav, 34, uint16(bits))
	copy(wav[36:40], "data")
	putLE32(wav, 40, uint32(dataSize))
	return wav
}

func putLE16(b []byte, off int, v uint16) {
	b[off] = byte(v)
	b[off+1] = byte(v >> 8)
}

func putLE32(b []byte, off int, v uint32) {
	b[off] = byte(v)
	b[off+1] = byte(v >> 8)
	b[off+2] = byte(v >> 16)
	b[off+3] = byte(v >> 24)
}
