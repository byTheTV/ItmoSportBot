package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// DB — SQLite: пользователи; шаблоны автозаписи в recurring_templates + recurring_signed.
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
		`CREATE TABLE IF NOT EXISTS recurring_templates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			telegram_chat_id INTEGER NOT NULL,
			public_id TEXT NOT NULL,
			source_lesson_id INTEGER NOT NULL DEFAULT 0,
			building_id INTEGER NOT NULL DEFAULT 0,
			weekday INTEGER NOT NULL,
			time_slot_start TEXT NOT NULL,
			time_slot_end TEXT NOT NULL,
			section_name TEXT NOT NULL,
			room_name TEXT NOT NULL,
			teacher_fio TEXT NOT NULL,
			type_name TEXT NOT NULL,
			lesson_level_name TEXT NOT NULL,
			FOREIGN KEY (telegram_chat_id) REFERENCES users(telegram_chat_id) ON DELETE CASCADE,
			UNIQUE(telegram_chat_id, public_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recurring_templates_chat ON recurring_templates(telegram_chat_id)`,
		`CREATE TABLE IF NOT EXISTS recurring_signed (
			template_row_id INTEGER NOT NULL REFERENCES recurring_templates(id) ON DELETE CASCADE,
			lesson_id INTEGER NOT NULL,
			PRIMARY KEY (template_row_id, lesson_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.SQL.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	if err := migrateRecurringJSONToTables(db); err != nil {
		return fmt.Errorf("migrate recurring json: %w", err)
	}
	if _, err := db.SQL.Exec(`ALTER TABLE users ADD COLUMN telegram_username TEXT`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("migrate telegram_username: %w", err)
		}
	}
	if _, err := db.SQL.Exec(`ALTER TABLE users ADD COLUMN min_lead_hours INTEGER NOT NULL DEFAULT 36`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("migrate min_lead_hours: %w", err)
		}
	}
	return nil
}
