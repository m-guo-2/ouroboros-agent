package runner

import (
	"context"
	"fmt"
	"strings"

	"agent/internal/storage"
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
	msgs, err := storage.GetSessionMessages(request.SessionID, 200)
	if err != nil {
		return storage.AttachmentData{}, false
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, attachment := range msgs[i].Attachments {
			if attachment.ID == attachmentID {
				return attachment, true
			}
		}
	}
	return storage.AttachmentData{}, false
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
