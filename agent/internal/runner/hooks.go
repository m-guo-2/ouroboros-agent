package runner

import (
	"context"
	"strings"

	"agent/internal/logger"
	"agent/internal/storage"
)

type HookPayload struct {
	SessionID      string
	ScopeType      string
	ScopeKey       string
	ActivatedAtSeq int64
	Relation       string
}

// DispatchHooks matches hooks by event name and executes their actions.
// Callers decide where and when to invoke this; the function makes no
// assumptions about the triggering context.
func DispatchHooks(ctx context.Context, hooks []storage.Hook, event string, sessionID string) {
	DispatchHooksWithPayload(ctx, hooks, event, HookPayload{SessionID: sessionID})
}

func DispatchHooksWithPayload(ctx context.Context, hooks []storage.Hook, event string, payload HookPayload) {
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
			executeHookAction(ctx, action, event, payload)
		}
	}

	if matched > 0 {
		logger.Business(ctx, "hook 分发完成",
			"event", event,
			"matchedHooks", matched,
			"sessionId", payload.SessionID,
			"relation", payload.Relation,
		)
	}
}

func executeHookAction(ctx context.Context, action storage.HookAction, event string, payload HookPayload) {
	switch action.Type {
	case "activate_skill":
		if action.SkillID == "" {
			logger.Warn(ctx, "hook action activate_skill 缺少 skillId", "sessionId", payload.SessionID)
			return
		}
		scopeType := strings.TrimSpace(action.ScopeType)
		if scopeType == "" {
			scopeType = strings.TrimSpace(payload.ScopeType)
		}
		if scopeType == "" {
			scopeType = "session"
		}
		scopeKey := ""
		if scopeType != "session" {
			scopeKey = payload.ScopeKey
		}
		err := storage.ActivateSessionSkillWithOptions(storage.ActivateSessionSkillOptions{
			SessionID:          payload.SessionID,
			SkillID:            action.SkillID,
			Source:             "hook",
			SourceEvent:        event,
			ScopeType:          scopeType,
			ScopeKey:           scopeKey,
			ActivatedAtSeq:     payload.ActivatedAtSeq,
			ExpiresAfterEvents: action.ExpiresAfterEvents,
		})
		if err != nil {
			logger.Warn(ctx, "hook activate_skill 失败",
				"sessionId", payload.SessionID,
				"skillId", action.SkillID,
				"error", err.Error(),
			)
			return
		}
		logger.Business(ctx, "hook 激活 skill",
			"sessionId", payload.SessionID,
			"skillId", action.SkillID,
			"scopeType", scopeType,
			"scopeKey", scopeKey,
			"expiresAfterEvents", action.ExpiresAfterEvents,
		)
	default:
		logger.Warn(ctx, "未识别的 hook action type",
			"type", action.Type,
			"sessionId", payload.SessionID,
		)
	}
}
