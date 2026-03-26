package storage

import (
	"crypto/rand"
	"fmt"
	"time"
)

// ChannelGroup represents a group chat reported by a channel adapter.
type ChannelGroup struct {
	ID             string `json:"id"`
	AgentID        string `json:"agentId"`
	Channel        string `json:"channel"`
	ChannelGroupID string `json:"channelGroupId"`
	GroupName      string `json:"groupName"`
	Status         string `json:"status"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
}

// UpsertChannelGroup inserts or updates a channel group record.
// On conflict (agent_id, channel, channel_group_id), it updates group_name,
// status, and updated_at.
func UpsertChannelGroup(agentID, channel, channelGroupID, groupName, status string) error {
	if agentID == "" || channel == "" || channelGroupID == "" {
		return fmt.Errorf("agentId, channel, and channelGroupId are required")
	}
	if status == "" {
		status = "active"
	}
	now := time.Now().UnixMilli()
	id := fmt.Sprintf("cg-%x%d", randGroupBytes(6), now%1e6)

	_, err := DB.Exec(`
		INSERT INTO channel_groups (id, agent_id, channel, channel_group_id, group_name, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, channel, channel_group_id) DO UPDATE SET
			group_name = CASE WHEN excluded.group_name != '' THEN excluded.group_name ELSE channel_groups.group_name END,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, id, agentID, channel, channelGroupID, groupName, status, now, now)
	return err
}

// GetChannelGroup retrieves a channel group by its unique key.
func GetChannelGroup(agentID, channel, channelGroupID string) (*ChannelGroup, error) {
	row := DB.QueryRow(`
		SELECT id, agent_id, channel, channel_group_id, group_name, status, created_at, updated_at
		FROM channel_groups
		WHERE agent_id = ? AND channel = ? AND channel_group_id = ?
	`, agentID, channel, channelGroupID)

	var g ChannelGroup
	err := row.Scan(&g.ID, &g.AgentID, &g.Channel, &g.ChannelGroupID, &g.GroupName, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// ListChannelGroups returns all groups for a given agent, optionally filtered by channel.
func ListChannelGroups(agentID, channel string) ([]ChannelGroup, error) {
	query := `SELECT id, agent_id, channel, channel_group_id, group_name, status, created_at, updated_at
		FROM channel_groups WHERE agent_id = ?`
	args := []any{agentID}
	if channel != "" {
		query += ` AND channel = ?`
		args = append(args, channel)
	}
	query += ` ORDER BY updated_at DESC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []ChannelGroup
	for rows.Next() {
		var g ChannelGroup
		if err := rows.Scan(&g.ID, &g.AgentID, &g.Channel, &g.ChannelGroupID, &g.GroupName, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func randGroupBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
