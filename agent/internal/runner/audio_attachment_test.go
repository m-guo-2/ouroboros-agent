package runner

import (
	"context"
	"testing"

	"agent/internal/storage"
)

func TestIsAudioFileAttachmentDistinguishesAudioFileFromVoiceMessage(t *testing.T) {
	audioFile := storage.AttachmentData{
		ID:          "att-audio",
		Kind:        "file",
		DisplayName: "meeting.m4a",
		MIMEType:    "audio/mp4",
		ResourceURI: "oss://bucket/meeting.m4a",
	}
	if !isAudioFileAttachment(audioFile) {
		t.Fatal("expected audio file attachment to be accepted")
	}

	voiceMessage := storage.AttachmentData{
		ID:                "att-voice",
		Kind:              "voice",
		DisplayName:       "voice.amr",
		MIMEType:          "audio/amr",
		SourceMessageType: "voice",
		ResourceURI:       "oss://bucket/voice.amr",
	}
	if !isVoiceMessageAttachment(voiceMessage) {
		t.Fatal("expected voice message attachment to be identified")
	}
	if isAudioFileAttachment(voiceMessage) {
		t.Fatal("voice message should not be accepted as an audio file attachment")
	}

	docFile := storage.AttachmentData{
		ID:          "att-doc",
		Kind:        "file",
		DisplayName: "report.pdf",
		MIMEType:    "application/pdf",
		ResourceURI: "oss://bucket/report.pdf",
	}
	if isAudioFileAttachment(docFile) {
		t.Fatal("document file should not be accepted as audio")
	}

	spxFile := storage.AttachmentData{
		ID:          "att-spx",
		Kind:        "file",
		DisplayName: "recording.spx",
		ResourceURI: "oss://bucket/recording.spx",
	}
	if !isAudioFileAttachment(spxFile) {
		t.Fatal("spx file should be accepted as audio")
	}
}

func TestTranscribeAudioAttachmentExecutorCallsParserForAudioFiles(t *testing.T) {
	req := ProcessRequest{Attachments: []storage.AttachmentData{{
		ID:          "att-audio",
		Kind:        "file",
		DisplayName: "meeting.wav",
		MIMEType:    "audio/wav",
		ResourceURI: "oss://bucket/meeting.wav",
	}}}
	var gotInput map[string]interface{}
	exec := createTranscribeAudioAttachmentExecutorWithParser(req, func(_ context.Context, input map[string]interface{}) (interface{}, error) {
		gotInput = input
		return map[string]interface{}{"text": "会议转写"}, nil
	})

	result, err := exec(context.Background(), map[string]interface{}{"attachmentId": "att-audio"})
	if err != nil {
		t.Fatalf("executor returned error: %v", err)
	}
	payload := result.(map[string]any)
	if payload["status"] != "ok" || payload["text"] != "会议转写" {
		t.Fatalf("unexpected result: %+v", payload)
	}
	if gotInput["messageType"] != "audio" || gotInput["resourceUri"] != "oss://bucket/meeting.wav" {
		t.Fatalf("unexpected parse input: %+v", gotInput)
	}
}

func TestTranscribeAudioAttachmentExecutorRejectsVoiceMessages(t *testing.T) {
	req := ProcessRequest{Attachments: []storage.AttachmentData{{
		ID:                "att-voice",
		Kind:              "voice",
		DisplayName:       "voice.amr",
		MIMEType:          "audio/amr",
		SourceMessageType: "voice",
		ResourceURI:       "oss://bucket/voice.amr",
	}}}
	exec := createTranscribeAudioAttachmentExecutorWithParser(req, func(_ context.Context, _ map[string]interface{}) (interface{}, error) {
		t.Fatal("parse executor should not be called for voice messages")
		return nil, nil
	})

	result, err := exec(context.Background(), map[string]interface{}{"attachmentId": "att-voice"})
	if err != nil {
		t.Fatalf("executor returned error: %v", err)
	}
	payload := result.(map[string]any)
	if payload["status"] != "failed" || payload["code"] != "voice_message_not_supported" {
		t.Fatalf("unexpected result: %+v", payload)
	}
}
