package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"
)

// ResolveUser finds or creates a shadow user for a channel identity.
// Returns (userID, isNew, error).
func ResolveUser(channelType, channelUserID, displayName string) (string, bool, error) {
	var userID string
	err := DB.QueryRow(
		`SELECT user_id FROM user_channels WHERE channel_type = ? AND channel_user_id = ?`,
		channelType, channelUserID,
	).Scan(&userID)

	if err == nil {
		return userID, false, nil
	}
	if err != sql.ErrNoRows {
		return "", false, fmt.Errorf("query user_channels: %w", err)
	}

	// No existing binding — create a new shadow user.
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	userID = fmt.Sprintf("u-%x", b)

	name := displayName
	if name == "" {
		name = fmt.Sprintf("%s:%s", channelType, channelUserID)
	}

	if _, err = DB.Exec(
		`INSERT OR IGNORE INTO users (id, name, type) VALUES (?, ?, 'human')`,
		userID, name,
	); err != nil {
		return "", false, fmt.Errorf("insert user: %w", err)
	}

	b2 := make([]byte, 6)
	_, _ = rand.Read(b2)
	bindingID := fmt.Sprintf("uc-%x", b2)
	if _, err = DB.Exec(
		`INSERT OR IGNORE INTO user_channels (id, user_id, channel_type, channel_user_id, display_name) VALUES (?, ?, ?, ?, ?)`,
		bindingID, userID, channelType, channelUserID, displayName,
	); err != nil {
		return "", false, fmt.Errorf("insert user_channel: %w", err)
	}

	return userID, true, nil
}
