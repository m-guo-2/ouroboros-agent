package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

var cst = time.FixedZone("CST", 8*3600)

// textToMs converts a TEXT timestamp to Unix milliseconds.
// Handles: CURRENT_TIMESTAMP format ("2006-01-02 15:04:05"), RFC3339, empty, already numeric.
func textToMs(s string) int64 {
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 {
			return n
		}
		if n > 1e9 {
			return n * 1000
		}
		return n
	}
	formats := []struct {
		layout string
		loc    *time.Location
	}{
		{time.RFC3339, nil},
		{time.RFC3339Nano, nil},
		{"2006-01-02T15:04:05", cst},
		{"2006-01-02 15:04:05", time.UTC}, // SQLite CURRENT_TIMESTAMP is UTC
		{"2006-01-02T15:04", cst},
		{"2006-01-02 15:04", time.UTC},
	}
	for _, f := range formats {
		var t time.Time
		var err error
		if f.loc != nil {
			t, err = time.ParseInLocation(f.layout, s, f.loc)
		} else {
			t, err = time.Parse(f.layout, s)
		}
		if err == nil {
			return t.UnixMilli()
		}
	}
	log.Printf("  ⚠ 无法解析时间: %q, 返回 0", s)
	return 0
}

type colMigration struct {
	table   string
	columns []string
}

var migrations = []colMigration{
	{"settings", []string{"updated_at"}},
	{"agent_configs", []string{"created_at", "updated_at"}},
	{"agent_sessions", []string{"created_at", "updated_at"}},
	{"users", []string{"created_at", "updated_at"}},
	{"user_channels", []string{"created_at"}},
	{"user_memory", []string{"updated_at"}},
	{"user_memory_facts", []string{"created_at", "expires_at"}},
	{"processed_messages", []string{"processed_at"}},
	{"messages", []string{"created_at"}},
	{"models", []string{"created_at", "updated_at"}},
	{"skills", []string{"created_at", "updated_at"}},
	{"context_compactions", []string{"archived_before_time", "created_at"}},
	{"delayed_tasks", []string{"execute_at", "created_at", "updated_at"}},
	{"session_facts", []string{"created_at"}},
}

func tableExists(db *sql.DB, table string) bool {
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
	return err == nil
}

func columnIsText(db *sql.DB, table, col string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var cname, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if cname == col && ctype == "TEXT" {
			return true
		}
	}
	return false
}

func migrateColumn(db *sql.DB, table, col string) (int, error) {
	if !columnIsText(db, table, col) {
		return 0, nil
	}

	tmpCol := col + "_new"

	stmts := []string{
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s INTEGER NOT NULL DEFAULT 0", table, tmpCol),
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return 0, fmt.Errorf("exec %q: %w", s[:60], err)
		}
	}

	rows, err := db.Query(fmt.Sprintf("SELECT rowid, %s FROM %s", col, table))
	if err != nil {
		return 0, fmt.Errorf("select %s.%s: %w", table, col, err)
	}

	type row struct {
		rowid int64
		val   int64
	}
	var updates []row
	for rows.Next() {
		var rid int64
		var raw sql.NullString
		if err := rows.Scan(&rid, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		s := ""
		if raw.Valid {
			s = raw.String
		}
		updates = append(updates, row{rid, textToMs(s)})
	}
	rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(fmt.Sprintf("UPDATE %s SET %s = ? WHERE rowid = ?", table, tmpCol))
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	for _, u := range updates {
		if _, err := stmt.Exec(u.val, u.rowid); err != nil {
			stmt.Close()
			tx.Rollback()
			return 0, err
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	// SQLite 不支持 DROP COLUMN（老版本），用重建表的方式太重了。
	// 简单方案：把新列的值写回老列（现在老列还是 TEXT 但存的是数字字符串），
	// 然后新代码能正常 Scan 成 int64。
	if _, err := db.Exec(fmt.Sprintf("UPDATE %s SET %s = %s", table, col, tmpCol)); err != nil {
		return 0, fmt.Errorf("copy back %s.%s: %w", table, col, err)
	}

	return len(updates), nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "用法: go run main.go <数据库路径>\n")
		fmt.Fprintf(os.Stderr, "示例: go run main.go ../../data/agent.db\n")
		os.Exit(1)
	}
	dbPath := os.Args[1]

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatalf("数据库不存在: %s", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		log.Fatalf("设置 busy_timeout: %v", err)
	}

	log.Printf("🔧 开始迁移: %s", dbPath)
	totalRows := 0

	for _, m := range migrations {
		if !tableExists(db, m.table) {
			log.Printf("  ⏭ 表 %s 不存在，跳过", m.table)
			continue
		}
		for _, col := range m.columns {
			n, err := migrateColumn(db, m.table, col)
			if err != nil {
				log.Printf("  ❌ %s.%s 迁移失败: %v", m.table, col, err)
				continue
			}
			if n == 0 {
				log.Printf("  ✅ %s.%s 已是 INTEGER 或无数据，跳过", m.table, col)
			} else {
				log.Printf("  ✅ %s.%s 转换 %d 行", m.table, col, n)
				totalRows += n
			}
		}
	}

	// 清理临时列（如果 SQLite 版本支持 DROP COLUMN >= 3.35.0）
	for _, m := range migrations {
		if !tableExists(db, m.table) {
			continue
		}
		for _, col := range m.columns {
			tmpCol := col + "_new"
			_, _ = db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", m.table, tmpCol))
		}
	}

	log.Printf("🎉 迁移完成，共转换 %d 行数据", totalRows)
	log.Printf("💡 注意: TEXT 列的值已被替换为数字字符串，新代码可以正常 Scan 为 int64")
}
