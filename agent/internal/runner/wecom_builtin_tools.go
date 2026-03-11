package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"agent/internal/config"
	"agent/internal/engine"
	"agent/internal/storage"
	"agent/internal/types"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

func registerWecomBuiltinTools(registry *engine.ToolRegistry, request ProcessRequest) {
	if request.Channel != "qiwei" {
		return
	}

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
		"向指定企微联系人或群聊主动发送消息。支持 text、rich_text、image、file、voice。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"channelConversationId": map[string]interface{}{"type": "string", "description": "群聊 ID（群消息时填写）"},
				"channelUserId":         map[string]interface{}{"type": "string", "description": "联系人 ID（私聊时填写）"},
				"messageType":           map[string]interface{}{"type": "string", "description": "消息类型：text（默认）/ rich_text / image / file / voice"},
				"content":               map[string]interface{}{"type": "string", "description": "消息内容。text/rich_text 填文字或富文本内容；image/file/voice 填可访问 URL"},
				"channelMeta":           map[string]interface{}{"type": "object", "description": "附加信息，如 file 类型时传 {\"fileName\": \"报告.pdf\"}"},
			},
			Required: []string{"content"},
		},
		createWecomHTTPToolExecutor("send_message"),
	)
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
