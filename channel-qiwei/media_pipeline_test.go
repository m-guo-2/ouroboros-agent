package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sharedoss "github.com/m-guo-2/ouroboros-agent/shared/oss"
	"github.com/youthlin/silk"
)

type fakeRecognizer struct {
	transcript      string
	imageText       string
	docText         string
	submitAudioData []byte
	submitFormat    string
}

type fakeMediaStorage struct {
	bucket string
	store  *sharedoss.FakeStorage
}

func newFakeMediaStorage(bucket string) *fakeMediaStorage {
	return &fakeMediaStorage{
		bucket: bucket,
		store: &sharedoss.FakeStorage{
			Objects:        make(map[string]sharedoss.FakeObject),
			PresignBaseURL: "https://signed.example.com",
		},
	}
}

func (s *fakeMediaStorage) PutObject(ctx context.Context, input sharedoss.PutObjectInput) (sharedoss.PutObjectResult, error) {
	result, err := s.store.PutObject(ctx, input)
	if err != nil {
		return sharedoss.PutObjectResult{}, err
	}
	result.Bucket = s.bucket
	return result, nil
}

func (s *fakeMediaStorage) GetObject(ctx context.Context, key string) (*sharedoss.GetObjectResult, error) {
	result, err := s.store.GetObject(ctx, key)
	if err != nil {
		return nil, err
	}
	result.Bucket = s.bucket
	return result, nil
}

func (s *fakeMediaStorage) PresignGetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return s.store.PresignGetURL(ctx, key, expiry)
}

func (f *fakeRecognizer) ParseImage(_ context.Context, attachment parsedAttachment) (parsedAttachment, error) {
	attachment.ParsedText = f.imageText
	return attachment, nil
}

func (f *fakeRecognizer) ParseDocument(_ context.Context, attachment parsedAttachment) (parsedAttachment, error) {
	attachment.ParsedText = f.docText
	return attachment, nil
}

func (f *fakeRecognizer) SubmitAudioTranscription(_ context.Context, audioData []byte, audioFormat string) (string, error) {
	f.submitAudioData = audioData
	f.submitFormat = audioFormat
	return "task-1", nil
}

func (f *fakeRecognizer) QueryAudioTranscription(_ context.Context, _ string) (parsedAttachment, bool, error) {
	return parsedAttachment{ParsedText: f.transcript}, true, nil
}

func TestClassifyMediaMessageDistinguishesSources(t *testing.T) {
	qw, ok := classifyMediaMessage(14, "", nil)
	if !ok || qw.Source != mediaSourceQW || qw.Kind != mediaKindImage {
		t.Fatalf("expected qw image classification, got %+v ok=%v", qw, ok)
	}

	gw, ok := classifyMediaMessage(101, "", nil)
	if !ok || gw.Source != mediaSourceGW || gw.Kind != mediaKindImage {
		t.Fatalf("expected gw image classification, got %+v ok=%v", gw, ok)
	}
}

func TestNormalizeMediaDescriptorNormalizesVariants(t *testing.T) {
	msgType, msgData := loadFixture(t, "gw-image.json")

	desc, err := normalizeMediaDescriptor(msgType, "image", msgData)
	if err != nil {
		t.Fatalf("normalizeMediaDescriptor failed: %v", err)
	}
	if desc.FileAESKey != "aes-gw-image" || desc.FileAuthKey != "auth-gw-image" {
		t.Fatalf("expected normalized keys, got %+v", desc)
	}
	if desc.Name != "测试.png" {
		t.Fatalf("expected decoded file name, got %q", desc.Name)
	}
	if desc.PreferredURL == "" || !strings.Contains(desc.PreferredURL, "tpdownloadmedia") {
		t.Fatalf("expected preferred gw url, got %q", desc.PreferredURL)
	}
}

func TestPlanMediaDownloadUsesSourceSpecificContracts(t *testing.T) {
	gwDesc := mediaDescriptor{
		Classification: mediaClassification{MessageType: "image", Source: mediaSourceGW, Kind: mediaKindImage},
		FileAESKey:     "aes",
		FileAuthKey:    "auth",
		FileSize:       128,
		FileType:       1,
		PreferredURL:   "https://imunion.weixin.qq.com/cgi-bin/mmae-bin/tpdownloadmedia?param=abc",
	}
	gwPlan, err := planMediaDownload(gwDesc)
	if err != nil {
		t.Fatalf("planMediaDownload gw failed: %v", err)
	}
	if gwPlan.Method != "/cloud/wxDownload" {
		t.Fatalf("expected gw download method, got %s", gwPlan.Method)
	}

	qwDesc := mediaDescriptor{
		Classification: mediaClassification{MessageType: "voice", Source: mediaSourceQW, Kind: mediaKindVoice},
		FileID:         "file-id",
		FileAESKey:     "aes",
		FileSize:       256,
		FileType:       5,
	}
	qwPlan, err := planMediaDownload(qwDesc)
	if err != nil {
		t.Fatalf("planMediaDownload qw failed: %v", err)
	}
	if qwPlan.Method != "/cloud/wxWorkDownload" {
		t.Fatalf("expected qw download method, got %s", qwPlan.Method)
	}
}

func TestParseMessageVoiceReturnsTranscript(t *testing.T) {
	app := newTestApp(t, "语音转写结果")
	msgType, msgData := loadFixture(t, "qw-voice.json")
	raw := map[string]any{"msgType": msgType}

	parsed, err := app.parseMessage(context.Background(), "voice", msgData, raw, "")
	if err != nil {
		t.Fatalf("parseMessage failed: %v", err)
	}
	if parsed.Text != "语音转写结果" {
		t.Fatalf("expected transcript text, got %q", parsed.Text)
	}
	if len(parsed.Attachments) != 0 {
		t.Fatalf("expected no attachment details for agent, got %+v", parsed.Attachments)
	}
}

func TestPrepareMediaForAgentVoiceSubmitsBase64DataForASR(t *testing.T) {
	app := newTestApp(t, "语音转写结果")
	msgType, msgData := loadFixture(t, "qw-voice.json")

	prepared := app.prepareMediaForAgent(context.Background(), msgType, "voice", msgData)
	if prepared.Content != "语音转写结果" {
		t.Fatalf("expected transcript content, got %q", prepared.Content)
	}

	recognizer, ok := app.recognizer.(*fakeRecognizer)
	if !ok {
		t.Fatalf("expected fakeRecognizer, got %T", app.recognizer)
	}
	if len(recognizer.submitAudioData) == 0 {
		t.Fatal("expected ASR to receive audio data bytes")
	}
	if recognizer.submitFormat != "wav" {
		t.Fatalf("expected wav format after silk conversion, got %q", recognizer.submitFormat)
	}
	if string(recognizer.submitAudioData[:4]) != "RIFF" {
		t.Fatal("expected wav RIFF header in submitted audio data")
	}
}

func TestParseMessageLocalImageUsesRecognizer(t *testing.T) {
	app := newTestApp(t, "")
	app.recognizer = &fakeRecognizer{imageText: "图片里的文字"}
	resourceURI := putTestObject(t, app, "sample.png", "image/png", []byte("fake image"))

	parsed, err := app.parseMessage(context.Background(), "image", nil, nil, resourceURI)
	if err != nil {
		t.Fatalf("parseMessage resource image failed: %v", err)
	}
	if parsed.Text != "图片里的文字" {
		t.Fatalf("expected parsed image text, got %q", parsed.Text)
	}
}

func TestParseMessageLocalFileReadsText(t *testing.T) {
	app := newTestApp(t, "")
	resourceURI := putTestObject(t, app, "sample.txt", "text/plain; charset=utf-8", []byte("这是文件内容"))

	parsed, err := app.parseMessage(context.Background(), "file", nil, nil, resourceURI)
	if err != nil {
		t.Fatalf("parseMessage resource file failed: %v", err)
	}
	if parsed.Text != "这是文件内容" {
		t.Fatalf("expected resource file text, got %q", parsed.Text)
	}
}

func TestHandleCallbackMessageForwardsPublicMediaAddress(t *testing.T) {
	app, getReceived := newTestAppWithAgentCapture(t, "")
	msgType, msgData := loadFixture(t, "gw-image.json")
	msg := qiweiCallbackMessage{
		GUID:           "guid",
		MsgType:        msgType,
		MsgData:        msgData,
		SenderID:       "user-1",
		SenderNickname: "Tester",
		MsgSvrID:       "msg-1",
		CreateTime:     123456,
	}

	if err := app.handleCallbackMessage(context.Background(), msg); err != nil {
		t.Fatalf("handleCallbackMessage failed: %v", err)
	}
	received := getReceived()
	if !strings.Contains(received.Content, "Tester: [收到图片]") {
		t.Fatalf("expected image placeholder, got %q", received.Content)
	}
	if len(received.Attachments) != 1 {
		t.Fatalf("expected one forwarded attachment, got %+v", received.Attachments)
	}
	if received.Attachments[0].Kind != "image" {
		t.Fatalf("expected image attachment, got %+v", received.Attachments[0])
	}
	if !strings.HasPrefix(received.Attachments[0].ResourceURI, "https://public.example.com/media/") {
		t.Fatalf("expected forwarded public url in attachment, got %+v", received.Attachments[0])
	}
	if received.ChannelMeta != nil {
		t.Fatalf("expected no leaked channel meta, got %+v", received.ChannelMeta)
	}
}

func TestHandleCallbackMessageFormatsVoiceTranscriptEvent(t *testing.T) {
	app, getReceived := newTestAppWithAgentCapture(t, "语音转写结果")
	msgType, msgData := loadFixture(t, "qw-voice.json")
	msg := qiweiCallbackMessage{
		GUID:           "guid",
		MsgType:        msgType,
		MsgData:        msgData,
		SenderID:       "user-1",
		SenderNickname: "Tester",
		MsgSvrID:       "msg-voice-1",
		CreateTime:     123456,
	}

	if err := app.handleCallbackMessage(context.Background(), msg); err != nil {
		t.Fatalf("handleCallbackMessage failed: %v", err)
	}
	received := getReceived()
	want := "Tester(用户)发送了语音消息，转换为文字是：语音转写结果"
	if received.Content != want {
		t.Fatalf("expected voice event %q, got %q", want, received.Content)
	}
	if len(received.Attachments) != 0 {
		t.Fatalf("expected no attachments for voice message, got %+v", received.Attachments)
	}
}

func testSilkBytes(t *testing.T) []byte {
	t.Helper()
	pcm := make([]byte, 24000*2)
	for i := range pcm {
		pcm[i] = byte(i % 256)
	}
	silkData, err := silk.Encode(bytes.NewReader(pcm))
	if err != nil {
		t.Fatalf("encode test silk data: %v", err)
	}
	return silkData
}

func newTestApp(t *testing.T, transcript string) *app {
	t.Helper()
	silkPayload := testSilkBytes(t)
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".silk") {
			w.Header().Set("Content-Type", "audio/silk")
			_, _ = w.Write(silkPayload)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("payload"))
		}
	}))
	t.Cleanup(downloadServer.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req qiweiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode qiwei request: %v", err)
		}
		resp := qiweiDoAPIResponse{
			Code: 0,
			Msg:  "成功",
			Data: json.RawMessage(`{"cloudUrl":"` + downloadServer.URL + `/asset.silk"}`),
		}
		if strings.Contains(req.Method, "cdnWxDownload") {
			resp.Data = json.RawMessage(`{"fileUrl":"` + downloadServer.URL + `/asset","coverUrl":"` + downloadServer.URL + `/cover"}`)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(apiServer.Close)

	cfg := Config{
		APIBaseURL:       apiServer.URL,
		Token:            "token",
		GUID:             "guid",
		AgentEnabled:     false,
		AgentServer:      "http://127.0.0.1",
		RequestTimout:    5,
		OSSPublicBaseURL: "https://public.example.com/media",
	}
	app := newApp(cfg)
	app.recognizer = &fakeRecognizer{transcript: transcript}
	app.storage = newFakeMediaStorage("test-bucket")
	return app
}

func putTestObject(t *testing.T, app *app, fileName, contentType string, body []byte) string {
	t.Helper()
	result, err := app.storage.PutObject(context.Background(), sharedoss.PutObjectInput{
		FileName:    fileName,
		ContentType: contentType,
		Size:        int64(len(body)),
		Body:        bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("put test object failed: %v", err)
	}
	return formatObjectURI(result.Bucket, result.Key)
}

func newTestAppWithAgentCapture(t *testing.T, transcript string) (*app, func() incomingMessage) {
	t.Helper()
	var (
		mu       sync.Mutex
		received incomingMessage
	)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode incoming message: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(agentServer.Close)

	app := newTestApp(t, transcript)
	app.cfg.AgentEnabled = true
	app.cfg.AgentServer = agentServer.URL

	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
	})
	return app, func() incomingMessage {
		mu.Lock()
		defer mu.Unlock()
		return received
	}
}

func TestDecodeSilkToWavProducesValidWav(t *testing.T) {
	silkData := testSilkBytes(t)
	if !isSilkData(silkData) {
		t.Fatal("expected test data to be detected as silk")
	}
	wavData, err := decodeSilkToWav(silkData)
	if err != nil {
		t.Fatalf("decodeSilkToWav failed: %v", err)
	}
	if len(wavData) < 44 {
		t.Fatalf("wav output too small: %d bytes", len(wavData))
	}
	if string(wavData[:4]) != "RIFF" {
		t.Fatalf("expected RIFF header, got %q", string(wavData[:4]))
	}
	if string(wavData[8:12]) != "WAVE" {
		t.Fatalf("expected WAVE marker, got %q", string(wavData[8:12]))
	}
}

func TestReplaceExtToWav(t *testing.T) {
	cases := []struct{ in, want string }{
		{"voice.silk", "voice.wav"},
		{"abc123.SILK", "abc123.wav"},
		{"audio.slk", "audio.wav"},
		{"noext", "noext.wav"},
	}
	for _, c := range cases {
		if got := replaceExtToWav(c.in); got != c.want {
			t.Errorf("replaceExtToWav(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func loadFixture(t *testing.T, name string) (int, map[string]any) {
	t.Helper()
	path := filepath.Join("testdata", "media", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var payload struct {
		MsgType int            `json:"msgType"`
		MsgData map[string]any `json:"msgData"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return payload.MsgType, payload.MsgData
}
