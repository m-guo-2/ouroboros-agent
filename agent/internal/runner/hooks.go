package runner

import (
	"context"

	"agent/internal/logger"
	"agent/internal/storage"
)

// DispatchHooks matches hooks by event name and executes their actions.
// Callers decide where and when to invoke this; the function makes no
// assumptions about the triggering context.
func DispatchHooks(ctx context.Context, hooks []storage.Hook, event string, sessionID string) {
	if len(hooks) == 0 {
		return
	}

	var matched int
	for _, h := range hooks {
		if h.Event != event {
			continue
		}
		matched++
		for _, action := range h.Actions {
			executeHookAction(ctx, action, sessionID)
		}
	}

	if matched > 0 {
		logger.Business(ctx, "hook 分发完成",
			"event", event,
			"matchedHooks", matched,
			"sessionId", sessionID,
		)
	}
}

func executeHookAction(ctx context.Context, action storage.HookAction, sessionID string) {
	switch action.Type {
	case "activate_skill":
		if action.SkillID == "" {
			logger.Warn(ctx, "hook action activate_skill 缺少 skillId", "sessionId", sessionID)
			return
		}
		if err := storage.ActivateSessionSkill(sessionID, action.SkillID, "hook"); err != nil {
			logger.Warn(ctx, "hook activate_skill 失败",
				"sessionId", sessionID,
				"skillId", action.SkillID,
				"error", err.Error(),
			)
			return
		}
		logger.Business(ctx, "hook 激活 skill",
			"sessionId", sessionID,
			"skillId", action.SkillID,
		)
	default:
		logger.Warn(ctx, "未识别的 hook action type",
			"type", action.Type,
			"sessionId", sessionID,
		)
	}
}
