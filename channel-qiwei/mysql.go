package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"channel-qiwei/migrations"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
)

// OpenMySQL opens the channel-qiwei MySQL connection, configures the
// pool, and runs goose.Up against the embedded migrations. Any failure
// is fatal to the caller.
func OpenMySQL(section MySQLSection) (*sql.DB, error) {
	dsn, redacted, err := buildQiweiDSN(section)
	if err != nil {
		return nil, fmt.Errorf("build mysql dsn: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(defaultInt(section.MaxOpenConns, 16))
	db.SetMaxIdleConns(defaultInt(section.MaxIdleConns, 8))
	if section.ConnMaxLifetime != "" {
		if d, err := time.ParseDuration(section.ConnMaxLifetime); err == nil {
			db.SetConnMaxLifetime(d)
		}
	} else {
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql %s: %w", redacted, err)
	}

	goose.SetBaseFS(migrations.FS)
	goose.SetTableName("goose_db_version")
	if err := goose.SetDialect("mysql"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("goose up: %w", err)
	}

	log.Printf("📦 MySQL connected: %s", redacted)
	return db, nil
}

func buildQiweiDSN(cfg MySQLSection) (dsn string, redacted string, err error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(cfg.Port)
	if port == "" {
		port = "3306"
	}
	user := strings.TrimSpace(cfg.User)
	if user == "" {
		return "", "", fmt.Errorf("mysql user is required")
	}
	db := strings.TrimSpace(cfg.Database)
	if db == "" {
		return "", "", fmt.Errorf("mysql database is required")
	}
	params := strings.TrimSpace(cfg.Params)
	if params == "" {
		params = "charset=utf8mb4&collation=utf8mb4_bin&loc=UTC&multiStatements=true"
	}

	// See agent/internal/storage/mysql.go for the rationale: go-sql-driver
	// does not percent-decode user/password, so we only escape the
	// characters that confuse its DSN tokenizer.
	auth := escapeDSNUser(user)
	if cfg.Password != "" {
		auth += ":" + escapeDSNPassword(cfg.Password)
	}
	addr := net.JoinHostPort(host, port)

	dsn = fmt.Sprintf("%s@tcp(%s)/%s?%s", auth, addr, db, params)

	maskedAuth := escapeDSNUser(user)
	if cfg.Password != "" {
		maskedAuth += ":****"
	}
	redacted = fmt.Sprintf("%s@tcp(%s)/%s?%s", maskedAuth, addr, db, params)
	return dsn, redacted, nil
}

func escapeDSNUser(s string) string {
	r := strings.NewReplacer(":", "%3A", "@", "%40")
	return r.Replace(s)
}

func escapeDSNPassword(s string) string {
	r := strings.NewReplacer("@", "%40", "?", "%3F")
	return r.Replace(s)
}

func defaultInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
