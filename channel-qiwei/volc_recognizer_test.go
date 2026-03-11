package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestVolcengineRecognizerParseImageUsesArkChatCompletion(t *testing.T) {
	var (
		gotAuth  string
		gotModel string
		gotImage string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = anyToString(body["model"])
		messages, _ := body["messages"].([]any)
		message, _ := messages[0].(map[string]any)
		content, _ := message["content"].([]any)
		imagePart, _ := content[1].(map[string]any)
		imageURL, _ := imagePart["image_url"].(map[string]any)
		gotImage = anyToString(imageURL["url"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"image summary"}}]}`))
	}))
	defer server.Close()

	imagePath := writeVisionFixture(t)
	recognizer := &volcengineRecognizer{
		cfg: Config{
			VolcArkBaseURL:  server.URL,
			VolcArkAPIKey:   "test-key",
			VolcVisionModel: "vision-model",
		},
		httpClient: server.Client(),
	}

	parsed, err := recognizer.ParseImage(context.Background(), parsedAttachment{
		Kind:      "image",
		Name:      filepath.Base(imagePath),
		LocalPath: imagePath,
	})
	if err != nil {
		t.Fatalf("ParseImage failed: %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
	if gotModel != "vision-model" {
		t.Fatalf("unexpected model: %q", gotModel)
	}
	if !strings.HasPrefix(gotImage, "data:image/png;base64,") {
		t.Fatalf("expected data url image payload, got %q", gotImage)
	}
	if parsed.ParsedText != "image summary" || parsed.ParseProvider != "volc-image" {
		t.Fatalf("unexpected parsed image result: %+v", parsed)
	}
}

func TestVolcengineRecognizerParseDocumentUsesArkChatCompletion(t *testing.T) {
	var (
		gotAuth   string
		gotModel  string
		gotPrompt string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = anyToString(body["model"])
		messages, _ := body["messages"].([]any)
		message, _ := messages[0].(map[string]any)
		gotPrompt = anyToString(message["content"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"document summary"}}]}`))
	}))
	defer server.Close()

	docPath := writeDocumentFixture(t)
	recognizer := &volcengineRecognizer{
		cfg: Config{
			VolcArkBaseURL:    server.URL,
			VolcArkAPIKey:     "test-key",
			VolcDocumentModel: "document-model",
		},
		httpClient: server.Client(),
	}

	parsed, err := recognizer.ParseDocument(context.Background(), parsedAttachment{
		Kind:      "document",
		Name:      filepath.Base(docPath),
		LocalPath: docPath,
	})
	if err != nil {
		t.Fatalf("ParseDocument failed: %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
	if gotModel != "document-model" {
		t.Fatalf("unexpected model: %q", gotModel)
	}
	if !strings.Contains(gotPrompt, "项目周报") {
		t.Fatalf("expected prompt to contain document text, got %q", gotPrompt)
	}
	if parsed.ParsedText != "document summary" || parsed.ParseProvider != "volc-document" {
		t.Fatalf("unexpected parsed document result: %+v", parsed)
	}
}

func TestVolcSpeechRecognizerSubmitAndQuery(t *testing.T) {
	var submitRequestID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submit":
			if got := r.Header.Get("X-Api-App-Key"); got != "app-key" {
				t.Fatalf("unexpected app key: %q", got)
			}
			if got := r.Header.Get("X-Api-Access-Key"); got != "access-key" {
				t.Fatalf("unexpected access key: %q", got)
			}
			if got := r.Header.Get("X-Api-Resource-Id"); got != "resource-id" {
				t.Fatalf("unexpected resource id: %q", got)
			}
			submitRequestID = r.Header.Get("X-Api-Request-Id")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode submit request: %v", err)
			}
			audio, _ := body["audio"].(map[string]any)
			if anyToString(audio["data"]) == "" {
				t.Fatal("expected base64 audio data in submit payload")
			}
			if anyToString(audio["format"]) != "wav" {
				t.Fatalf("unexpected audio format: %q", anyToString(audio["format"]))
			}
			w.Header().Set("X-Api-Status-Code", "20000000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case "/query":
			if got := r.Header.Get("X-Api-Request-Id"); got != submitRequestID {
				t.Fatalf("unexpected query request id: %q", got)
			}
			w.Header().Set("X-Api-Status-Code", "20000000")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"text":"speech summary"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	recognizer := &volcengineRecognizer{
		cfg: Config{
			VolcSpeechAppKey:     "app-key",
			VolcSpeechAccessKey:  "access-key",
			VolcSpeechResourceID: "resource-id",
			VolcSpeechSubmitURL:  server.URL + "/submit",
			VolcSpeechQueryURL:   server.URL + "/query",
		},
		httpClient: server.Client(),
	}

	taskID, err := recognizer.SubmitAudioTranscription(context.Background(), []byte("fake-wav-data"), "wav")
	if err != nil {
		t.Fatalf("SubmitAudioTranscription failed: %v", err)
	}
	if strings.TrimSpace(taskID) == "" {
		t.Fatal("expected non-empty task id")
	}

	parsed, done, err := recognizer.QueryAudioTranscription(context.Background(), taskID)
	if err != nil {
		t.Fatalf("QueryAudioTranscription failed: %v", err)
	}
	if !done {
		t.Fatal("expected transcription query to finish")
	}
	if parsed.ParsedText != "speech summary" || parsed.ParseProvider != "volc-speech" {
		t.Fatalf("unexpected speech parse result: %+v", parsed)
	}
}
