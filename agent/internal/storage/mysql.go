package storage

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"agent/internal/storage/migrations"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
)

// MySQLConfig carries just the connection parameters the storage layer
// needs. Callers map their own config struct into this shape.
type MySQLConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Database        string
	Params          string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime string
}

// InitMySQL opens the MySQL connection, configures the pool, and runs
// goose.Up against the embedded migrations. It assigns the global DB on
// success. Any failure is fatal to the caller (returns error; callers
// log.Fatal on error).
func InitMySQL(cfg MySQLConfig) error {
	dsn, redacted, err := BuildDSN(cfg)
	if err != nil {
		return fmt.Errorf("build mysql dsn: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(defaultInt(cfg.MaxOpenConns, 16))
	db.SetMaxIdleConns(defaultInt(cfg.MaxIdleConns, 8))
	if cfg.ConnMaxLifetime != "" {
		if d, err := time.ParseDuration(cfg.ConnMaxLifetime); err == nil {
			db.SetConnMaxLifetime(d)
		}
	} else {
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping mysql %s: %w", redacted, err)
	}

	goose.SetBaseFS(migrations.FS)
	goose.SetTableName("goose_db_version")
	if err := goose.SetDialect("mysql"); err != nil {
		_ = db.Close()
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		_ = db.Close()
		return fmt.Errorf("goose up: %w", err)
	}

	DB = db
	if err := RecoverInterruptedExecutions(); err != nil {
		_ = db.Close()
		DB = nil
		return fmt.Errorf("recover interrupted executions: %w", err)
	}
	log.Printf("📦 MySQL connected: %s", redacted)
	return nil
}

// BuildDSN assembles a go-sql-driver/mysql DSN. It returns both the real
// DSN (with password) and a version safe for logs (password masked).
func BuildDSN(cfg MySQLConfig) (dsn string, redacted string, err error) {
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

	// go-sql-driver/mysql does NOT percent-decode user/password from the
	// DSN; it splits on the first ':' (between user and password) and the
	// last '@' (between password and host). Per its docs, only '@' and
	// '?' inside the password (or ':' / '@' in the username) need to be
	// escaped, and the escape is the literal-character form expected by
	// the driver, not URL query encoding. Treat the rest of the password
	// (e.g. '%', '&', '*') as raw bytes — escaping them silently corrupts
	// the credential and yields confusing 1045 access-denied errors.
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

// escapeDSNUser escapes only the characters that confuse go-sql-driver's
// DSN parser: ':' (user/password separator) and '@' (auth/host separator).
func escapeDSNUser(s string) string {
	r := strings.NewReplacer(":", "%3A", "@", "%40")
	return r.Replace(s)
}

// escapeDSNPassword escapes only '@' and '?' per go-sql-driver docs.
// '@' would terminate the auth section; '?' would prematurely start
// query params.
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
