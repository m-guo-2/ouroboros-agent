package runner

import (
	"testing"

	"agent/internal/storage"
	"agent/internal/types"
)

func TestShouldRequireAttachmentInspection(t *testing.T) {
	imageAttachments := []storage.AttachmentData{{ID: "att-1", Kind: "image", ResourceURI: "oss://bucket/a.png"}}
	fileAttachments := []storage.AttachmentData{{ID: "att-2", Kind: "file", ResourceURI: "oss://bucket/a.pdf"}}

	if !shouldRequireAttachmentInspection("帮我看看这张图里是什么", imageAttachments) {
		t.Fatal("expected image question to require inspection")
	}
	if shouldRequireAttachmentInspection("收到，谢谢", imageAttachments) {
		t.Fatal("did not expect casual acknowledgement to require inspection")
	}
	if !shouldRequireAttachmentInspection("帮我总结一下这个文件内容", fileAttachments) {
		t.Fatal("expected file summary request to require inspection")
	}
}

func TestHasAttachmentInspectionUse(t *testing.T) {
	messages := []types.AgentMessage{
		{
			Role: "assistant",
			Content: []types.ContentBlock{{
				Type:  "tool_use",
				Name:  "inspect_attachment",
				Input: map[string]interface{}{"attachmentId": "att-1"},
			}},
		},
	}
	if !hasAttachmentInspectionUse(messages, []string{"att-1"}) {
		t.Fatal("expected matching inspect_attachment tool use to be detected")
	}
	if hasAttachmentInspectionUse(messages, []string{"att-2"}) {
		t.Fatal("did not expect non-matching attachment id to be detected")
	}
}
