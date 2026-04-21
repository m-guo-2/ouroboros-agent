package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// seedFromYAML writes one default account to the DB if the table is empty
// and the legacy YAML top-level (guid/token) is populated. This keeps single
// account deployments working without any admin API call on upgrade.
//
// Returns the number of rows seeded (0 or 1) and an error.
func seedFromYAML(ctx context.Context, db *sql.DB, cfg Config) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM qiwei_accounts`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count accounts: %w", err)
	}
	if count > 0 {
		return 0, nil
	}
	guid := strings.TrimSpace(cfg.GUID)
	token := strings.TrimSpace(cfg.Token)
	if guid == "" || token == "" {
		return 0, nil
	}

	repo := newAccountRepo(db)
	display := firstNonEmpty(cfg.AgentID, "default")
	_, err := repo.CreateAccount(ctx, Account{
		GUID:        guid,
		Token:       token,
		DisplayName: display,
		AgentID:     cfg.AgentID,
		Enabled:     true,
		Notes:       "seeded from config.yaml on first boot",
	})
	if err != nil {
		return 0, fmt.Errorf("seed default account: %w", err)
	}
	return 1, nil
}

// migrateKnownRoomsFile pulls the legacy flat known_rooms.txt into the DB
// and archives the file as known_rooms.txt.migrated.{unix}. Idempotent: if
// the file is missing or already archived, it's a no-op.
// Returns the number of rooms migrated.
func migrateKnownRoomsFile(ctx context.Context, db *sql.DB, accountID, dataDir string) (int, error) {
	path := filepath.Join(dataDir, "known_rooms.txt")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var ids []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			ids = append(ids, line)
		}
	}
	_ = f.Close()
	if len(ids) == 0 {
		archived := path + fmt.Sprintf(".migrated.%d", time.Now().Unix())
		_ = os.Rename(path, archived)
		return 0, nil
	}

	rs := newRoomStore(db, accountID)
	rs.Merge(ids)

	archived := path + fmt.Sprintf(".migrated.%d", time.Now().Unix())
	if err := os.Rename(path, archived); err != nil {
		return len(ids), fmt.Errorf("archive legacy known_rooms: %w", err)
	}
	return len(ids), nil
}
