package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"agent/internal/storage"
	"agent/internal/types"
)

func createInspectAttachmentExecutor(request ProcessRequest) func(context.Context, map[string]interface{}) (interface{}, error) {
	parseExecutor := createWecomHTTPToolExecutor("parse_message")
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		attachmentID := strings.TrimSpace(anyString(input["attachmentId"]))
		if attachmentID == "" {
			return map[string]any{
				"status":  "failed",
				"code":    "attachment_id_required",
				"message": "attachmentId is required",
			}, nil
		}
		goal := strings.TrimSpace(anyString(input["goal"]))
		if goal == "" {
			return map[string]any{
				"status":       "failed",
				"code":         "analysis_goal_required",
				"attachmentId": attachmentID,
				"message":      "goal is required",
			}, nil
		}
		attachment, ok := resolveAttachmentForSession(request, attachmentID)
		if !ok {
			return map[string]any{
				"status":       "failed",
				"code":         "attachment_not_found",
				"attachmentId": attachmentID,
				"message":      "attachment not found in current session",
			}, nil
		}
		task := normalizeAttachmentTask(anyString(input["task"]), attachment.Kind)
		if code, message := validateAttachmentTask(task, attachment.Kind); code != "" {
			return map[string]any{
				"status":       "failed",
				"code":         code,
				"attachmentId": attachmentID,
				"task":         task,
				"message":      message,
			}, nil
		}
		result, err := parseExecutor(ctx, map[string]interface{}{
			"messageType": attachment.Kind,
			"resourceUri": attachment.ResourceURI,
			"goal":        goal,
		})
		if err != nil {
			return map[string]any{
				"status":       "failed",
				"code":         classifyAttachmentInspectionError(err),
				"attachmentId": attachmentID,
				"task":         task,
				"message":      err.Error(),
			}, nil
		}

		payload, _ := result.(map[string]interface{})
		text := strings.TrimSpace(anyString(payload["text"]))
		if text == "" {
			text = strings.TrimSpace(anyString(payload["content"]))
		}
		return map[string]any{
			"status":       "ok",
			"attachmentId": attachmentID,
			"kind":         attachment.Kind,
			"task":         task,
			"goal":         goal,
			"text":         text,
			"raw":          result,
		}, nil
	}
}

func createTranscribeAudioAttachmentExecutor(request ProcessRequest) types.ToolExecutor {
	return createTranscribeAudioAttachmentExecutorWithParser(request, createWecomHTTPToolExecutor("parse_message"))
}

func createTranscribeAudioAttachmentExecutorWithParser(request ProcessRequest, parseExecutor types.ToolExecutor) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		attachmentID := strings.TrimSpace(anyString(input["attachmentId"]))
		if attachmentID == "" {
			return map[string]any{
				"status":  "failed",
				"code":    "attachment_id_required",
				"message": "attachmentId is required",
			}, nil
		}
		attachment, ok := resolveAttachmentForSession(request, attachmentID)
		if !ok {
			return map[string]any{
				"status":       "failed",
				"code":         "attachment_not_found",
				"attachmentId": attachmentID,
				"message":      "attachment not found in current session",
			}, nil
		}
		if isVoiceMessageAttachment(attachment) {
			return map[string]any{
				"status":       "failed",
				"code":         "voice_message_not_supported",
				"attachmentId": attachmentID,
				"kind":         attachment.Kind,
				"message":      "voice messages are transcribed before they reach the agent; this tool only handles user-sent audio files",
			}, nil
		}
		if !isAudioFileAttachment(attachment) {
			return map[string]any{
				"status":       "failed",
				"code":         "not_audio_file",
				"attachmentId": attachmentID,
				"kind":         attachment.Kind,
				"mimeType":     attachment.MIMEType,
				"name":         attachment.DisplayName,
				"message":      "attachment is not an audio file",
			}, nil
		}
		result, err := parseExecutor(ctx, map[string]interface{}{
			"messageType": "audio",
			"resourceUri": attachment.ResourceURI,
		})
		if err != nil {
			return map[string]any{
				"status":       "failed",
				"code":         classifyAttachmentInspectionError(err),
				"attachmentId": attachmentID,
				"message":      err.Error(),
			}, nil
		}
		payload, _ := result.(map[string]interface{})
		text := strings.TrimSpace(anyString(payload["text"]))
		if text == "" {
			text = strings.TrimSpace(anyString(payload["content"]))
		}
		return map[string]any{
			"status":       "ok",
			"attachmentId": attachmentID,
			"kind":         attachment.Kind,
			"mimeType":     attachment.MIMEType,
			"name":         attachment.DisplayName,
			"text":         text,
			"raw":          result,
		}, nil
	}
}

func resolveAttachmentForSession(request ProcessRequest, attachmentID string) (storage.AttachmentData, bool) {
	for _, attachment := range request.Attachments {
		if attachment.ID == attachmentID {
			return attachment, true
		}
	}
	if request.SessionID == "" {
		return storage.AttachmentData{}, false
	}
	return storage.FindAttachmentInSession(request.SessionID, attachmentID)
}

func normalizeAttachmentTask(task, kind string) string {
	task = strings.TrimSpace(task)
	if task != "" {
		return task
	}
	switch strings.TrimSpace(kind) {
	case "image":
		return "describe_image"
	case "file":
		return "extract_text"
	case "video":
		return "summarize_video"
	default:
		return ""
	}
}

func validateAttachmentTask(task, kind string) (string, string) {
	switch strings.TrimSpace(kind) {
	case "image":
		switch task {
		case "describe_image", "ocr_image":
			return "", ""
		}
	case "file":
		switch task {
		case "extract_text", "summarize_document":
			return "", ""
		}
	case "video":
		switch task {
		case "summarize_video":
			return "unsupported_format", "video analysis is not implemented yet"
		}
	default:
		return "unsupported_format", fmt.Sprintf("attachment kind %q is not supported", kind)
	}
	return "invalid_task", fmt.Sprintf("task %q is not valid for %s attachment", task, kind)
}

func isVoiceMessageAttachment(attachment storage.AttachmentData) bool {
	return strings.TrimSpace(attachment.Kind) == "voice" || strings.TrimSpace(attachment.SourceMessageType) == "voice"
}

func isAudioFileAttachment(attachment storage.AttachmentData) bool {
	if strings.TrimSpace(attachment.Kind) != "file" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(attachment.MIMEType)), "audio/") {
		return true
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.TrimSpace(attachment.DisplayName))), ".")
	if ext == "" {
		ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.TrimSpace(attachment.ResourceURI))), ".")
	}
	return isSupportedOrConvertibleAudioExt(ext)
}

func isSupportedOrConvertibleAudioExt(ext string) bool {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".") {
	case "mp3", "wav", "m4a", "aac", "ogg", "oga", "opus", "spx", "amr", "silk", "slk", "flac", "webm", "weba":
		return true
	default:
		return false
	}
}

func classifyAttachmentInspectionError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not configured"):
		return "provider_unconfigured"
	case strings.Contains(msg, "download failed"):
		return "download_failed"
	case strings.Contains(msg, "timed out"), strings.Contains(msg, "timeout"):
		return "analysis_timeout"
	case strings.Contains(msg, "unsupported"):
		return "unsupported_format"
	default:
		return "analysis_failed"
	}
}

func anyString(v interface{}) string {
	s, _ := v.(string)
	return s
}
