package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"fmt"
	"os"
	"strings"
)

// OpenDB is the test entry point. Production code uses OpenMySQL directly.
//
// The legacy SQLite path has been removed; tests that still pass a file path
// now route through MySQL using TEST_MYSQL_* environment variables. When no
// test-MySQL is configured the function returns an error so tests can t.Skip.
func OpenDB(path string) (*sql.DB, error) {
	section, ok := testMySQLSectionFromEnv()
	if !ok {
		return nil, fmt.Errorf("OpenDB: SQLite removed and TEST_MYSQL_* env not set (path=%q)", path)
	}
	return OpenMySQL(section)
}

// testMySQLSectionFromEnv assembles a MySQLSection from TEST_MYSQL_* env
// variables if present. Returns ok=false when required fields are missing so
// callers can skip gracefully.
func testMySQLSectionFromEnv() (MySQLSection, bool) {
	host := os.Getenv("TEST_MYSQL_HOST")
	user := os.Getenv("TEST_MYSQL_USER")
	db := os.Getenv("TEST_MYSQL_QIWEI_DATABASE")
	if host == "" || user == "" || db == "" {
		return MySQLSection{}, false
	}
	return MySQLSection{
		Host:     host,
		Port:     envOr("TEST_MYSQL_PORT", "3306"),
		User:     user,
		Password: os.Getenv("TEST_MYSQL_PASSWORD"),
		Database: db,
		Params:   "charset=utf8mb4&collation=utf8mb4_bin&loc=UTC&multiStatements=true",
	}, true
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// deriveShortHash returns a URL-safe 8-char short hash of the given guid.
// Uses SHA-256 → first 5 bytes → base32 (no padding, lowercase).
// 5 bytes → 8 chars of base32, giving us a 40-bit space (collision probability
// negligible for our expected account count).
func deriveShortHash(guid string) string {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(guid))
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:5])
	return strings.ToLower(enc)
}
