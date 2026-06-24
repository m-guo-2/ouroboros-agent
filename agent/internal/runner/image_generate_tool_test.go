package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent/internal/storage"

	sharedoss "github.com/m-guo-2/ouroboros-agent/shared/oss"
)

func TestReferenceAttachmentDataURLReadsOSSObject(t *testing.T) {
	raw := []byte("fake png bytes")
	store := sharedoss.NewFakeStorage()
	store.Objects["incoming/reference.png"] = sharedoss.FakeObject{
		Body:        raw,
		ContentType: "image/png",
	}

	got, err := referenceAttachmentDataURL(context.Background(), store, storage.AttachmentData{
		ID:          "att-1",
		Kind:        "image",
		ResourceURI: "oss://media/incoming/reference.png",
		MIMEType:    "image/png",
	})
	if err != nil {
		t.Fatalf("referenceAttachmentDataURL returned error: %v", err)
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	if got != want {
		t.Fatalf("data URL = %q, want %q", got, want)
	}
}

func TestFirstGeneratedImageParsesSupportedShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		url  string
		b64  string
	}{
		{
			name: "openai url",
			body: `{"data":[{"url":"https://example.com/image.png"}]}`,
			url:  "https://example.com/image.png",
		},
		{
			name: "images array",
			body: `{"images":["https://example.com/from-images.png"]}`,
			url:  "https://example.com/from-images.png",
		},
		{
			name: "base64",
			body: `{"data":[{"b64_json":"aGVsbG8="}]}`,
			b64:  "aGVsbG8=",
		},
		{
			name: "download url",
			body: `{"data":{"download_url":"https://example.com/download.png"}}`,
			url:  "https://example.com/download.png",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := firstGeneratedImage([]byte(tc.body))
			if err != nil {
				t.Fatalf("firstGeneratedImage returned error: %v", err)
			}
			if got.URL != tc.url {
				t.Fatalf("URL = %q, want %q", got.URL, tc.url)
			}
			if got.B64JSON != tc.b64 {
				t.Fatalf("B64JSON = %q, want %q", got.B64JSON, tc.b64)
			}
		})
	}
}

func TestCallResponsesImageGenerationAPI(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	resp, err := callResponsesImageGenerationAPI(context.Background(), server.URL+"/v1", "test-key", responsesImageGenerationRequest{
		Model:           "doubao-seed-2-0-mini-260215",
		ImageModel:      "gpt-image-2",
		Prompt:          "test prompt",
		Size:            "1024x1024",
		ReferenceImages: []string{"https://example.com/reference.png"},
		ExtraParameters: map[string]interface{}{"quality": "high", "ignored": true},
	})
	if err != nil {
		t.Fatalf("callResponsesImageGenerationAPI returned error: %v", err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want /v1/responses", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["model"] != "doubao-seed-2-0-mini-260215" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	tools := gotBody["tools"].([]interface{})
	tool := tools[0].(map[string]interface{})
	if tool["type"] != "image_generation" || tool["model"] != "gpt-image-2" {
		t.Fatalf("tool = %#v", tool)
	}
	if tool["quality"] != "high" {
		t.Fatalf("quality = %v", tool["quality"])
	}
	if _, ok := tool["ignored"]; ok {
		t.Fatalf("unexpected ignored extra parameter in tool: %#v", tool)
	}
	if resp.B64JSON != "aGVsbG8=" {
		t.Fatalf("B64JSON = %q, want aGVsbG8=", resp.B64JSON)
	}
}

func TestCallImageGenerationAPI(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":["https://example.com/result.png"]}`))
	}))
	defer server.Close()

	resp, err := callImageGenerationAPI(context.Background(), server.URL+"/v1", "test-key", map[string]interface{}{
		"model":  defaultImageModel,
		"prompt": "test prompt",
		"size":   "2K",
	})
	if err != nil {
		t.Fatalf("callImageGenerationAPI returned error: %v", err)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path = %q, want /v1/images/generations", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["model"] != "doubao-seedream-5-0-260128" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	if len(resp) == 0 {
		t.Fatal("expected response bytes")
	}
}

func TestFirstResponsesGeneratedImageParsesCompletedJSON(t *testing.T) {
	image, err := firstResponsesGeneratedImage([]byte(`{"response":{"output":[{"type":"image_generation_call","result":"aGVsbG8="}]}}`))
	if err != nil {
		t.Fatalf("firstResponsesGeneratedImage returned error: %v", err)
	}
	if image.B64JSON != "aGVsbG8=" {
		t.Fatalf("B64JSON = %q, want aGVsbG8=", image.B64JSON)
	}
}

func TestNormalizeImageModelFallsBackForVolcengine(t *testing.T) {
	got := normalizeImageModel("volcengine", "gpt-image-2")
	if got != defaultImageModel {
		t.Fatalf("normalizeImageModel = %q, want %q", got, defaultImageModel)
	}
}

func TestNormalizeImageSizeFallsBackForVolcengineSmallSquare(t *testing.T) {
	got := normalizeImageSize("volcengine", "1024x1024")
	if got != defaultImageSize {
		t.Fatalf("normalizeImageSize = %q, want %q", got, defaultImageSize)
	}
}
