package storage

import "agent/internal/timeutil"

func IsProcessed(key string) bool {
	var dummy string
	err := DB.QueryRow(
		`SELECT channel_message_id FROM processed_messages WHERE channel_message_id = ?`, key,
	).Scan(&dummy)
	return err == nil
}

func MarkProcessed(key, channelType string) error {
	_, err := DB.Exec(
		`INSERT OR IGNORE INTO processed_messages (channel_message_id, channel_type, processed_at) VALUES (?, ?, ?)`,
		key, channelType, timeutil.NowMs(),
	)
	return err
}
