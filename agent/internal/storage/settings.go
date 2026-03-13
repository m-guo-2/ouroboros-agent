package storage

import (
	"database/sql"
	"fmt"

	"agent/internal/timeutil"
)

// GetSettingValue reads a single settings entry by key.
// Returns ("", nil) when the key does not exist.
func GetSettingValue(key string) (string, error) {
	var value string
	err := DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// GetAllSettings returns all key-value pairs in the settings table.
func GetAllSettings() (map[string]string, error) {
	rows, err := DB.Query("SELECT key, value FROM settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetSettingValue upserts a settings key.
func SetSettingValue(key, value string) error {
	now := timeutil.NowMs()
	_, err := DB.Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = ?`,
		key, value, now, now,
	)
	return err
}

// DeleteSettingValue removes a setting by key. Returns true if a row was deleted.
func DeleteSettingValue(key string) (bool, error) {
	res, err := DB.Exec("DELETE FROM settings WHERE key = ?", key)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetMultipleSettings upserts multiple settings in a single transaction.
func SetMultipleSettings(kv map[string]string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = ?`,
	)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for k, v := range kv {
		now := timeutil.NowMs()
		if _, err := stmt.Exec(k, v, now, now); err != nil {
			tx.Rollback()
			return fmt.Errorf("set %q: %w", k, err)
		}
	}
	return tx.Commit()
}
