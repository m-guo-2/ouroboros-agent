package runner

import (
	"context"
	"fmt"
	"strings"

	"agent/internal/channels"
	"agent/internal/engine"
	"agent/internal/logger"
	"agent/internal/types"
)

const planModePromptSuffix = `

你当前处于计划模式。用户希望你先制定方案再执行，不要直接操作。

规则：
- 只使用只读工具收集信息（wecom_search_targets、wecom_list_or_get_conversations、wecom_get_group_detail、wecom_get_contact_detail、inspect_attachment）
- 禁止使用任何会产生副作用的工具（wecom_send_message、wecom_revoke_message、send_channel_message、execute_command 等）
- 充分理解任务和收集必要信息后，制定清晰的执行计划
- 计划就绪后调用 exit_plan_mode 工具将计划发送给用户审批`

func registerPlanModeTools(registry *engine.ToolRegistry, worker *SessionWorker, sessionReq ProcessRequest) {
	registry.RegisterBuiltin("enter_plan_mode",
		`进入计划模式——先制定方案、获得用户确认后再执行。

何时使用：
- 操作影响多个目标（多个群、多个联系人、批量操作）
- 不可逆的高风险操作（批量发消息、群成员管理、好友处理）
- 用户请求模糊或有歧义，需要先确认范围和方案
- 涉及敏感内容（清理群成员、转发隐私信息、批量通知）

不需要使用：
- 简单查询（"这个群有多少人"、"帮我看一下最近消息"）
- 单条消息回复
- 用户指令非常明确且影响范围小（"给张三发句早安"）`,
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"_": map[string]interface{}{"type": "string", "description": "忽略此字段。无参数工具的兼容占位字段"},
			},
		},
		createEnterPlanModeExecutor(worker),
	)

	registry.RegisterBuiltin("exit_plan_mode",
		"退出计划模式。将你制定的计划发送给用户审批，然后恢复正常模式。用户确认后你再按计划执行。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"plan": map[string]interface{}{"type": "string", "description": "完整的执行计划文本，将发送给用户审批"},
			},
			Required: []string{"plan"},
		},
		createExitPlanModeExecutor(worker, sessionReq),
	)
}

func createEnterPlanModeExecutor(worker *SessionWorker) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		if worker.Mode == types.SessionModePlan {
			return map[string]interface{}{
				"status":  "already_in_plan_mode",
				"message": "已在计划模式中，请继续收集信息并制定计划。",
			}, nil
		}
		worker.Mode = types.SessionModePlan
		logger.Business(ctx, "进入计划模式",
			"traceEvent", "mode_change",
			"sessionId", worker.SessionID,
			"from", "normal", "to", "plan")
		return map[string]interface{}{
			"status":  "entered_plan_mode",
			"message": "已进入计划模式。请使用只读工具收集信息、制定执行计划，完成后调用 exit_plan_mode 将计划发送给用户审批。",
		}, nil
	}
}

func createExitPlanModeExecutor(worker *SessionWorker, sessionReq ProcessRequest) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		if worker.Mode != types.SessionModePlan {
			return nil, fmt.Errorf("当前不在计划模式中，无需退出")
		}

		plan, _ := input["plan"].(string)
		if strings.TrimSpace(plan) == "" {
			return nil, fmt.Errorf("计划内容不能为空")
		}

		outMsg := channels.OutgoingMessage{
			Channel:               sessionReq.Channel,
			ChannelUserID:         sessionReq.ChannelUserID,
			Content:               plan,
			ChannelConversationID: sessionReq.ChannelConversationID,
			SessionID:             worker.SessionID,
			TraceID:               sessionReq.TraceID,
		}
		if err := channels.SendToChannel(outMsg); err != nil {
			return nil, fmt.Errorf("发送计划失败: %w", err)
		}

		worker.Mode = types.SessionModeNormal
		logger.Business(ctx, "退出计划模式，计划已发送",
			"traceEvent", "mode_change",
			"sessionId", worker.SessionID,
			"from", "plan", "to", "normal",
			"planLength", len(plan))
		return map[string]interface{}{
			"status":  "plan_sent",
			"message": "计划已发送给用户。等待用户回复确认或修改意见后，再按计划执行。",
		}, nil
	}
}
