package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agent/internal/config"
	"agent/internal/engine"
	"agent/internal/storage"
	"agent/internal/types"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

func registerWecomBuiltinTools(registry *engine.ToolRegistry, request ProcessRequest) {
	registry.RegisterBuiltin("wecom_search_targets",
		"搜索企微中的沟通对象，统一覆盖联系人和群聊。输入关键词后返回可直接沟通的 targets，每个结果都带 type、id、name。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"query":           map[string]interface{}{"type": "string", "description": "搜索关键词，可以是姓名、备注、手机号、企业名或群名。不填时返回默认列表"},
				"limit":           map[string]interface{}{"type": "integer", "description": "返回结果上限，默认 20"},
				"includeContacts": map[string]interface{}{"type": "boolean", "description": "是否搜索联系人，默认 true"},
				"includeGroups":   map[string]interface{}{"type": "boolean", "description": "是否搜索群聊，默认 true"},
			},
		},
		createWecomHTTPToolExecutor("search_targets"),
	)

	registry.RegisterBuiltin("wecom_list_or_get_conversations",
		"统一处理企微会话读取。不给 conversationId 时，返回最近会话列表；给了 conversationId 时，返回该会话的历史消息。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"conversationId": map[string]interface{}{"type": "string", "description": "目标会话 ID。留空时列最近会话；填写后读取该会话历史消息"},
				"msgSvrId":       map[string]interface{}{"type": "string", "description": "读取历史消息时的翻页起点。留空则从最新消息开始"},
				"currentSeq":     map[string]interface{}{"type": "number", "description": "列会话时的分页游标，首次传 0"},
				"pageSize":       map[string]interface{}{"type": "number", "description": "列会话时每页数量，默认使用服务端默认值"},
			},
		},
		createWecomHTTPToolExecutor("list_or_get_conversations"),
	)

	registry.RegisterBuiltin("wecom_parse_message",
		"解析企微消息内容，统一处理文本、图片、文件、语音等输入。可传原始 message、messageType+msgData，或直接传 qiwei 已准备好的 resourceUri；旧的 localPath 仍兼容。语音默认返回转写文本；图片/文件在拿到资源地址后可进一步解析内容。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"message":     map[string]interface{}{"type": "object", "description": "原始消息对象。推荐直接传 wecom_list_or_get_conversations 返回的某条 raw message"},
				"messageType": map[string]interface{}{"type": "string", "description": "消息类型。若未传 message，可单独传 text / image / file / voice / rich_text"},
				"msgData":     map[string]interface{}{"type": "object", "description": "消息载荷。若未传完整 message，可用这个字段传原始 msgData"},
				"resourceUri": map[string]interface{}{"type": "string", "description": "qiwei 已准备好的资源地址，例如 oss://bucket/key。适合对图片/文件做二次理解时直接传入"},
				"localPath":   map[string]interface{}{"type": "string", "description": "兼容旧参数，效果等同于 resourceUri"},
			},
		},
		createWecomHTTPToolExecutor("parse_message"),
	)

	registry.RegisterBuiltin("inspect_attachment",
		"按需分析当前会话中的结构化附件。优先传 attachmentId；图片可做 describe_image 或 ocr_image，文件可做 extract_text 或 summarize_document。语音已前置转写，不需要通过这个工具处理。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"attachmentId": map[string]interface{}{"type": "string", "description": "附件 ID。来自用户消息里 [attachments] 段中的 id 字段"},
				"task":         map[string]interface{}{"type": "string", "description": "分析任务：describe_image / ocr_image / extract_text / summarize_document / summarize_video"},
			},
			Required: []string{"attachmentId"},
		},
		createInspectAttachmentExecutor(request),
	)

	registry.RegisterBuiltin("wecom_send_message",
		`向指定企微联系人或群聊主动发送消息。支持单条（content）或多条（messages 数组，最多 4 条）。支持 text、rich_text、image、file、voice、link、location、miniapp。

调用规则：尽量一次说完，禁止连续调用本工具；只有中间穿插其他工具调用时才可分开发送。需要混发文字和图片/文件时，用 messages 数组在一次调用内完成。

风格：用微信闲聊的语气，口语化通俗易懂，小学文化水平也能理解。禁用序号（1.2.3.）、标题（# ##）、括号（【】[]）等结构化符号，禁止比喻，省略称谓和语气词，直接输出回复文本。正面回答问题让用户有获得感，提问一次最多两个问题，不垫话直接问。

富媒体：图片/文件/语音等需设置对应 messageType，不支持 Markdown 语法。每条 message 必须是完整段落，禁止把一句话拆成多条碎片。`,
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"channelConversationId": map[string]interface{}{"type": "string", "description": "群聊 ID（群消息时填写）"},
				"channelUserId":         map[string]interface{}{"type": "string", "description": "联系人 ID（私聊时填写）"},
				"messageType":           map[string]interface{}{"type": "string", "description": "单条模式的消息类型：text（默认）/ rich_text / image / file / voice / link / location / miniapp"},
				"content":               map[string]interface{}{"type": "string", "description": "单条消息内容（与 messages 二选一）。text/rich_text 填文字；image/file/voice 填可访问 URL；link 填链接地址；location/miniapp 可留空由 channelMeta 承载"},
				"messages": map[string]interface{}{
					"type":        "array",
					"description": "多条消息数组，最多 4 条，按顺序发送。与 content 二选一。适合文字+图片混发等场景",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"content":     map[string]interface{}{"type": "string", "description": "消息内容"},
							"messageType": map[string]interface{}{"type": "string", "description": "消息类型：text（默认）/ rich_text / image / file / voice / link / location / miniapp"},
							"channelMeta": map[string]interface{}{"type": "object", "description": "附加参数"},
						},
						"required": []string{"content"},
					},
				},
				"channelMeta": map[string]interface{}{"type": "object", "description": `单条模式的附加信息，按 messageType 使用：
- file: {"fileName": "报告.pdf"}
- link: {"title": "标题", "desc": "描述", "linkUrl": "https://...", "iconUrl": "图标URL"}
- location: {"title": "地点名", "address": "详细地址", "latitude": "纬度", "longitude": "经度"}
- miniapp: 透传 QiWei sendWeapp 所需全部参数
- text/rich_text 引用回复: {"reply": {"msgSvrId": "被引用消息ID", "content": "原消息摘要"}}`},
			},
		},
		createWecomSendMessageExecutor(),
	)

	registry.RegisterBuiltin("wecom_revoke_message",
		"撤回已发送的企微消息。需要 chatId（会话 ID）和 msgServerId（消息服务端 ID）。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"chatId":      map[string]interface{}{"type": "string", "description": "会话 ID，即消息所在的聊天对象 ID"},
				"msgServerId": map[string]interface{}{"type": "string", "description": "待撤回消息的服务端 ID"},
			},
			Required: []string{"chatId", "msgServerId"},
		},
		createWecomModuleActionExecutor("message", "revoke"),
	)
}

func createWecomSendMessageExecutor() types.ToolExecutor {
	sendOne := func(ctx context.Context, payload map[string]interface{}) (interface{}, error) {
		url := fmt.Sprintf("%s/api/qiwei/send_message", config.ResolveQiweiBaseURL(func(key string) string {
			v, _ := storage.GetSettingValue(key)
			return v
		}))

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		client := sharedlogger.NewClient("wecom-tool", 30*time.Second)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("wecom send_message failed: %d %s", resp.StatusCode, string(respBytes))
		}

		var result interface{}
		if err := json.Unmarshal(respBytes, &result); err == nil {
			return result, nil
		}
		return string(respBytes), nil
	}

	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		convID, _ := input["channelConversationId"].(string)
		userID, _ := input["channelUserId"].(string)

		type msgItem struct {
			content     string
			messageType string
			channelMeta map[string]interface{}
		}

		var items []msgItem

		if rawMessages, ok := input["messages"].([]interface{}); ok && len(rawMessages) > 0 {
			if len(rawMessages) > 4 {
				return nil, fmt.Errorf("messages 数组最多 4 条，当前 %d 条", len(rawMessages))
			}
			for i, raw := range rawMessages {
				m, ok := raw.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("messages[%d] 格式无效", i)
				}
				c, _ := m["content"].(string)
				if strings.TrimSpace(c) == "" {
					return nil, fmt.Errorf("messages[%d].content 不能为空", i)
				}
				mt, _ := m["messageType"].(string)
				cm, _ := m["channelMeta"].(map[string]interface{})
				items = append(items, msgItem{content: c, messageType: mt, channelMeta: cm})
			}
		} else {
			content, _ := input["content"].(string)
			if strings.TrimSpace(content) == "" {
				return nil, fmt.Errorf("content 或 messages 必须提供其一")
			}
			mt, _ := input["messageType"].(string)
			cm, _ := input["channelMeta"].(map[string]interface{})
			items = append(items, msgItem{content: content, messageType: mt, channelMeta: cm})
		}

		type sendResult struct {
			Index   int         `json:"index"`
			Success bool        `json:"success"`
			Error   string      `json:"error,omitempty"`
			Data    interface{} `json:"data,omitempty"`
		}
		results := make([]sendResult, 0, len(items))
		allOK := true

		for i, item := range items {
			payload := map[string]interface{}{
				"content": item.content,
			}
			if convID != "" {
				payload["channelConversationId"] = convID
			}
			if userID != "" {
				payload["channelUserId"] = userID
			}
			if item.messageType != "" {
				payload["messageType"] = item.messageType
			}
			if len(item.channelMeta) > 0 {
				payload["channelMeta"] = item.channelMeta
			}

			data, err := sendOne(ctx, payload)
			if err != nil {
				results = append(results, sendResult{Index: i, Success: false, Error: err.Error()})
				allOK = false
				continue
			}
			results = append(results, sendResult{Index: i, Success: true, Data: data})
		}

		if len(items) == 1 {
			if allOK {
				return results[0].Data, nil
			}
			return nil, fmt.Errorf("%s", results[0].Error)
		}
		return map[string]interface{}{"success": allOK, "results": results}, nil
	}
}

func createWecomModuleActionExecutor(module, action string) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		url := fmt.Sprintf("%s/api/qiwei/%s/%s", config.ResolveQiweiBaseURL(func(key string) string {
			v, _ := storage.GetSettingValue(key)
			return v
		}), module, action)

		body, err := json.Marshal(map[string]interface{}{"params": input})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		client := sharedlogger.NewClient("wecom-tool", 30*time.Second)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("wecom module action failed: %d %s", resp.StatusCode, string(respBytes))
		}

		var result interface{}
		if err := json.Unmarshal(respBytes, &result); err == nil {
			return result, nil
		}
		return string(respBytes), nil
	}
}

func createWecomHTTPToolExecutor(path string) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		url := fmt.Sprintf("%s/api/qiwei/%s", config.ResolveQiweiBaseURL(func(key string) string {
			v, _ := storage.GetSettingValue(key)
			return v
		}), path)

		body, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		client := sharedlogger.NewClient("wecom-tool", 30*time.Second)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("wecom builtin tool failed: %d %s", resp.StatusCode, string(respBytes))
		}

		var result interface{}
		if err := json.Unmarshal(respBytes, &result); err == nil {
			return result, nil
		}
		return string(respBytes), nil
	}
}
