package storage


// IsProcessed returns true when the given dedup key was already handled.
func IsProcessed(key string) bool {
	var dummy string
	err := DB.QueryRow(
		`SELECT channel_message_id FROM processed_messages WHERE channel_message_id = ?`, key,
	).Scan(&dummy)
	return err == nil
}

// MarkProcessed records a dedup key so future identical messages are skipped.
func MarkProcessed(key, channelType string) error {
	_, err := DB.Exec(
		`INSERT OR IGNORE INTO processed_messages (channel_message_id, channel_type) VALUES (?, ?)`,
		key, channelType,
	)
	return err
}
