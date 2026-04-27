// Command migrate-up applies the embedded goose migrations against the
// MySQL database described by environment variables, then exits.
//
// It exists for ops scenarios where you want to provision schema without
// also booting the full agent runtime (port listener, scheduler, …).
//
// Required env (same names the agent itself reads):
//
//	MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD,
//	AGENT_MYSQL_DATABASE  (default: moli_agent)
package main

import (
	"log"
	"os"

	"agent/internal/storage"
)

func main() {
	cfg := storage.MySQLConfig{
		Host:     getenv("MYSQL_HOST", "127.0.0.1"),
		Port:     getenv("MYSQL_PORT", "3306"),
		User:     os.Getenv("MYSQL_USER"),
		Password: os.Getenv("MYSQL_PASSWORD"),
		Database: getenv("AGENT_MYSQL_DATABASE", "moli_agent"),
		Params: getenv("AGENT_MYSQL_PARAMS",
			"charset=utf8mb4&collation=utf8mb4_bin&parseTime=true&loc=UTC&multiStatements=true"),
	}
	if err := storage.Init(cfg); err != nil {
		log.Fatalf("migrate-up: %v", err)
	}
	if err := storage.SeedDefaults(); err != nil {
		log.Fatalf("migrate-up: seed: %v", err)
	}
	log.Printf("migrate-up: schema + seeds applied to %s", cfg.Database)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
