package migrator

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// newID returns a 24-char hex string used as the synthetic primary key
// for child rows that the legacy SQLite store did not assign one to.
func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ---------------------------------------------------------------
// agent_skill_bindings: source agent_configs.skills accepts both
// new flat (["skill-a", ...]) and old object ([{"id":"x","mode":"y"}])
// shapes.
// ---------------------------------------------------------------

type skillBinding struct {
	id   string
	mode string
}

func parseSkillBindings(raw string) ([]skillBinding, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}
	var flat []string
	if err := json.Unmarshal([]byte(raw), &flat); err == nil {
		out := make([]skillBinding, 0, len(flat))
		for _, id := range flat {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			out = append(out, skillBinding{id: id})
		}
		return out, nil
	}
	var objs []struct {
		ID   string `json:"id"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(raw), &objs); err != nil {
		return nil, fmt.Errorf("not an array of strings or objects: %v (raw=%q)", err, raw)
	}
	out := make([]skillBinding, 0, len(objs))
	for _, o := range objs {
		o.ID = strings.TrimSpace(o.ID)
		if o.ID == "" {
			continue
		}
		out = append(out, skillBinding{id: o.ID, mode: strings.TrimSpace(o.Mode)})
	}
	return out, nil
}

func insertAgentSkillBindings(ctx context.Context, tx *sql.Tx, agentID, raw string, createdAt int64) (int64, error) {
	skills, err := parseSkillBindings(raw)
	if err != nil {
		return 0, fmt.Errorf("agent %s skills: %w", agentID, err)
	}
	for i, sk := range skills {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO agent_skill_bindings
			(id, agent_id, skill_id, mode, position, created_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, 0)`,
			newID(), agentID, sk.id, sk.mode, i, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(skills)), nil
}

// ---------------------------------------------------------------
// agent_channels: source shape is
//   [{"channelType":"webui","channelIdentifier":"*"}, ...]
// We also accept the legacy ["webui", ...] flat form (identifier="*").
// ---------------------------------------------------------------

func insertAgentChannels(ctx context.Context, tx *sql.Tx, agentID, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return 0, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return 0, fmt.Errorf("agent %s channels: %w", agentID, err)
	}
	written := int64(0)
	for i, item := range arr {
		ct, ident := "", "*"
		var s string
		if err := json.Unmarshal(item, &s); err == nil && s != "" {
			ct = s
		} else {
			var o map[string]string
			if err := json.Unmarshal(item, &o); err != nil {
				return 0, fmt.Errorf("agent %s channels[%d]: %w", agentID, i, err)
			}
			ct = firstNonEmpty(o["channelType"], o["type"])
			ident = firstNonEmpty(o["channelIdentifier"], o["identifier"], "*")
		}
		if ct == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO agent_channels
			(id, agent_id, channel_type, channel_identifier, position, created_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, 0)`,
			newID(), agentID, ct, ident, i, createdAt); err != nil {
			return 0, err
		}
		written++
	}
	return written, nil
}

// ---------------------------------------------------------------
// agent_hooks: source shape is
//   [{"event":"...", "actions":[{"type":"activate_skill","skillId":"..."}]}]
// We fan it out into one row per (event, action). Legacy single-action
// shape (no "actions" wrapper) is also handled.
// ---------------------------------------------------------------

type hookAction struct {
	Type    string `json:"type"`
	SkillID string `json:"skillId"`
}

type hookEntry struct {
	Event   string       `json:"event"`
	Actions []hookAction `json:"actions"`
	// legacy: single action embedded directly
	Type    string `json:"type"`
	SkillID string `json:"skillId"`
}

func parseAgentHookRows(raw string) ([]hookAction, []string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil, nil
	}
	var arr []hookEntry
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, nil, err
	}
	var actions []hookAction
	var events []string
	for _, h := range arr {
		event := h.Event
		if len(h.Actions) > 0 {
			for _, a := range h.Actions {
				actions = append(actions, a)
				events = append(events, event)
			}
			continue
		}
		if h.Type != "" || h.SkillID != "" {
			actions = append(actions, hookAction{Type: h.Type, SkillID: h.SkillID})
			events = append(events, event)
		}
	}
	return actions, events, nil
}

func countAgentHookRows(raw string) int64 {
	actions, _, err := parseAgentHookRows(raw)
	if err != nil {
		return 0
	}
	return int64(len(actions))
}

func insertAgentHooks(ctx context.Context, tx *sql.Tx, agentID, raw string, createdAt int64) (int64, error) {
	actions, events, err := parseAgentHookRows(raw)
	if err != nil {
		return 0, fmt.Errorf("agent %s hooks: %w", agentID, err)
	}
	for i, a := range actions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO agent_hooks
			(agent_id, event, action_type, skill_id, position, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			agentID, events[i], a.Type, a.SkillID, i, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(actions)), nil
}

// ---------------------------------------------------------------
// agent_subagent_models: { subKey: {provider, model} }
// ---------------------------------------------------------------

func insertAgentSubagentModels(ctx context.Context, tx *sql.Tx, agentID, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return 0, nil
	}
	var m map[string]struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return 0, fmt.Errorf("agent %s subagent_models: %w", agentID, err)
	}
	for key, v := range m {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO agent_subagent_models
			(id, agent_id, subagent_key, provider, model, created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
			newID(), agentID, key, v.Provider, v.Model, createdAt, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(m)), nil
}

// ---------------------------------------------------------------
// agent_subagent_skill_bindings: { subKey: [skillID, ...] }
// ---------------------------------------------------------------

func insertAgentSubagentSkills(ctx context.Context, tx *sql.Tx, agentID, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return 0, nil
	}
	var m map[string][]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return 0, fmt.Errorf("agent %s subagent_skills: %w", agentID, err)
	}
	var n int64
	for key, ids := range m {
		for i, id := range ids {
			if id == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO agent_subagent_skill_bindings
				(id, agent_id, subagent_key, skill_id, position, created_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, 0)`,
				newID(), agentID, key, id, i, createdAt); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, nil
}

// ---------------------------------------------------------------
// persona_skill_bindings / persona_subagent_models /
// persona_subagent_skill_bindings: same JSON shapes, but bound to
// persona_id rather than agent_id.
// ---------------------------------------------------------------

func insertPersonaSkillBindings(ctx context.Context, tx *sql.Tx, personaID, raw string, createdAt int64) (int64, error) {
	skills, err := parseSkillBindings(raw)
	if err != nil {
		return 0, fmt.Errorf("persona %s skills: %w", personaID, err)
	}
	for i, sk := range skills {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO persona_skill_bindings
			(id, persona_id, skill_id, mode, position, created_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, 0)`,
			newID(), personaID, sk.id, sk.mode, i, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(skills)), nil
}

func insertPersonaSubagentModels(ctx context.Context, tx *sql.Tx, personaID, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return 0, nil
	}
	var m map[string]struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return 0, fmt.Errorf("persona %s subagent_models: %w", personaID, err)
	}
	for key, v := range m {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO persona_subagent_models
			(id, persona_id, subagent_key, provider, model, created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
			newID(), personaID, key, v.Provider, v.Model, createdAt, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(m)), nil
}

func insertPersonaSubagentSkills(ctx context.Context, tx *sql.Tx, personaID, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return 0, nil
	}
	var m map[string][]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return 0, fmt.Errorf("persona %s subagent_skills: %w", personaID, err)
	}
	var n int64
	for key, ids := range m {
		for i, id := range ids {
			if id == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO persona_subagent_skill_bindings
				(id, persona_id, subagent_key, skill_id, position, created_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, 0)`,
				newID(), personaID, key, id, i, createdAt); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, nil
}

// ---------------------------------------------------------------
// message_tool_calls: extract tool_use blocks from messages.tool_calls.
// ---------------------------------------------------------------

func insertMessageToolCalls(ctx context.Context, tx *sql.Tx, messageID int64, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return 0, nil
	}
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return 0, fmt.Errorf("message %d tool_calls: %w", messageID, err)
	}
	var n int64
	for i, b := range blocks {
		typ, _ := b["type"].(string)
		if typ != "" && typ != "tool_use" {
			continue
		}
		toolCallID, _ := b["id"].(string)
		toolName, _ := b["name"].(string)
		if toolName == "" {
			if t, ok := b["tool"].(string); ok {
				toolName = t
			}
		}
		argsBytes, _ := json.Marshal(b["input"])
		resultBytes, _ := json.Marshal(b["result"])
		args := defaultJSONObject(string(argsBytes))
		result := string(resultBytes)
		if result == "null" {
			result = ""
		}
		status, _ := b["status"].(string)
		if status == "" {
			status = "pending"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO message_tool_calls
			(message_id, seq, tool_call_id, tool_name, arguments_json, result_json, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			messageID, i, toolCallID, toolName, args, result, status, createdAt); err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}

// ---------------------------------------------------------------
// message_attachments: source shape is
//   [{"id":"...", "kind":"image", "resourceUri":"...",
//     "displayName":"...", "mimeType":"...",
//     "sourceMessageType":"image"}]
// ---------------------------------------------------------------

func insertMessageAttachments(ctx context.Context, tx *sql.Tx, messageID int64, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return 0, nil
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return 0, fmt.Errorf("message %d attachments: %w", messageID, err)
	}
	for i, a := range arr {
		attachmentID, _ := a["id"].(string)
		kind, _ := a["kind"].(string)
		if kind == "" {
			kind, _ = a["type"].(string)
		}
		uri, _ := a["resourceUri"].(string)
		if uri == "" {
			uri, _ = a["uri"].(string)
		}
		if uri == "" {
			uri, _ = a["url"].(string)
		}
		display, _ := a["displayName"].(string)
		mime, _ := a["mimeType"].(string)
		sourceType, _ := a["sourceMessageType"].(string)
		if sourceType == "" {
			sourceType, _ = a["sourceType"].(string)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO message_attachments
			(message_id, seq, attachment_id, kind, resource_uri, display_name, mime_type, source_type, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			messageID, i, attachmentID, kind, uri, display, mime, sourceType, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(arr)), nil
}

// ---------------------------------------------------------------
// context_compaction_archived_messages: split
// context_compaction_archives.archived_messages JSON.
// ---------------------------------------------------------------

func insertArchivedMessages(ctx context.Context, tx *sql.Tx, archiveID int64, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return 0, nil
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return 0, fmt.Errorf("archive %d archived_messages: %w", archiveID, err)
	}
	for i, m := range arr {
		role, _ := m["role"].(string)
		msgType, _ := m["messageType"].(string)
		if msgType == "" {
			msgType, _ = m["message_type"].(string)
		}
		if msgType == "" {
			msgType = "text"
		}
		var origID int64
		switch v := m["originalMessageId"].(type) {
		case float64:
			origID = int64(v)
		case int64:
			origID = v
		}
		if origID == 0 {
			switch v := m["id"].(type) {
			case float64:
				origID = int64(v)
			case int64:
				origID = v
			}
		}
		// content can be string or structured; persist as string/JSON.
		content := ""
		switch c := m["content"].(type) {
		case string:
			content = c
		default:
			b, _ := json.Marshal(c)
			content = string(b)
			if content == "null" {
				content = ""
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO context_compaction_archived_messages
			(archive_id, seq, original_message_id, role, content, message_type, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			archiveID, i, origID, role, content, msgType, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(arr)), nil
}

// firstNonEmpty returns the first argument that is not the empty string.
// Used for legacy/new key fallbacks in JSON splits.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
