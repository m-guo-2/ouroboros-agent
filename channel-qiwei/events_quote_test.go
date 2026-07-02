package main

import (
	"strings"
	"testing"
)

func TestContentFromQuoteUsesReferMessageAliases(t *testing.T) {
	content, meta := contentFromQuote(map[string]any{
		"title": "收到，我看下",
		"refMsg": map[string]any{
			"msgid":       "ref-1001",
			"contentHtml": "被引用的正文",
			"senderName":  "张三",
		},
	})

	if content != "收到，我看下" {
		t.Fatalf("unexpected reply content: %q", content)
	}
	qm := quotedMeta(t, meta)
	if qm["msgSvrId"] != "ref-1001" {
		t.Fatalf("expected quoted msg id, got %#v", qm["msgSvrId"])
	}
	if qm["content"] != "被引用的正文" {
		t.Fatalf("expected quoted content, got %#v", qm["content"])
	}
	if qm["senderName"] != "张三" {
		t.Fatalf("expected quoted sender, got %#v", qm["senderName"])
	}
}

func TestContentFromQuoteUsesXMLReferMessage(t *testing.T) {
	content, meta := contentFromQuote(map[string]any{
		"content": `<msg><appmsg><title>这个怎么处理</title><type>57</type><refermsg><svrid>ref-2002</svrid><displayname>李四</displayname><content>原始问题</content></refermsg></appmsg></msg>`,
	})

	if content != "这个怎么处理" {
		t.Fatalf("unexpected reply content: %q", content)
	}
	qm := quotedMeta(t, meta)
	if qm["msgSvrId"] != "ref-2002" || qm["content"] != "原始问题" || qm["senderName"] != "李四" {
		t.Fatalf("unexpected quoted meta: %#v", qm)
	}
}

func TestContentFromQuoteDescribesReferencedFileFields(t *testing.T) {
	content, meta := contentFromQuote(map[string]any{
		"title": "看下这个文件",
		"referMsg": map[string]any{
			"msgSvrId":   "ref-file-1",
			"fileName":   "报价单.xlsx",
			"fileSize":   int64(2048),
			"fileUrl":    "https://example.com/file.xlsx",
			"senderName": "王五",
		},
	})

	if content != "看下这个文件" {
		t.Fatalf("unexpected reply content: %q", content)
	}
	qm := quotedMeta(t, meta)
	got, _ := qm["content"].(string)
	for _, want := range []string{"[引用文件]", "名称: 报价单.xlsx"} {
		if !strings.Contains(got, want) {
			t.Fatalf("quoted file content missing %q: %q", want, got)
		}
	}
	for _, noisy := range []string{"大小:", "地址:", "文件ID:"} {
		if strings.Contains(got, noisy) {
			t.Fatalf("quoted file content should not expose %q: %q", noisy, got)
		}
	}
}

func TestContentFromQuoteDescribesReferencedFileXML(t *testing.T) {
	content, meta := contentFromQuote(map[string]any{
		"title": "这份可以吗",
		"refermsg": map[string]any{
			"svrid":   "ref-file-xml",
			"content": `<msg><appmsg><title>需求文档.docx</title><type>6</type><appattach><attachid>attach-1</attachid><totallen>4096</totallen><fileext>docx</fileext></appattach></appmsg></msg>`,
		},
	})

	if content != "这份可以吗" {
		t.Fatalf("unexpected reply content: %q", content)
	}
	qm := quotedMeta(t, meta)
	got, _ := qm["content"].(string)
	for _, want := range []string{"[引用文件]", "名称: 需求文档.docx"} {
		if !strings.Contains(got, want) {
			t.Fatalf("quoted XML file content missing %q: %q", want, got)
		}
	}
	for _, noisy := range []string{"大小:", "文件ID:"} {
		if strings.Contains(got, noisy) {
			t.Fatalf("quoted XML file content should not expose %q: %q", noisy, got)
		}
	}
}

func quotedMeta(t *testing.T, meta map[string]any) map[string]any {
	t.Helper()
	qm, ok := meta["quotedMessage"].(map[string]any)
	if !ok {
		t.Fatalf("quotedMessage missing from meta: %#v", meta)
	}
	return qm
}
