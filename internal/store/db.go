package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// DB — SQLite: пользователи и JSON шаблонов на chat_id.
type DB struct {
	SQL *sql.DB
}

func Open(path string) (*DB, error) {
	path = filepath.Clean(path)
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0750)
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(1)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	db := &DB{SQL: sqlDB}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	if db == nil || db.SQL == nil {
		return nil
	}
	return db.SQL.Close()
}

func (db *DB) migrate() error {
	if _, err := db.SQL.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("migrate pragma: %w", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			telegram_chat_id INTEGER NOT NULL UNIQUE,
			display_name TEXT,
			priority INTEGER NOT NULL DEFAULT 100,
			refresh_token_enc BLOB,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS user_recurring (
			telegram_chat_id INTEGER PRIMARY KEY,
			templates_json TEXT NOT NULL DEFAULT '{"templates":[]}',
			FOREIGN KEY (telegram_chat_id) REFERENCES users(telegram_chat_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_priority ON users(priority ASC, id ASC)`,
	}
	for _, s := range stmts {
		if _, err := db.SQL.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	if _, err := db.SQL.Exec(`ALTER TABLE users ADD COLUMN telegram_username TEXT`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("migrate telegram_username: %w", err)
		}
	}
	return nil
}
