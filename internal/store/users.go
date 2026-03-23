package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"itmosportbot/internal/config"
)

// User — строка из БД для логики бота. Linked заполняется в ListAllUsers.
type User struct {
	ID                int64
	TelegramChatID    int64
	TelegramUsername  string // @username без @
	DisplayName       string
	Priority          int
	MinLeadHours      int // минимум часов до начала пары для автозаписи; 0 = не требовать
	CreatedAt         time.Time
	Linked            bool
}

// UpsertUser создаёт или обновляет пользователя; refreshToken пустой — не трогать поле токена.
func (db *DB) UpsertUser(chatID int64, displayName string, priority int, refreshToken *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if refreshToken != nil {
		enc, err := encryptToken(*refreshToken)
		if err != nil {
			return err
		}
		_, err = db.SQL.Exec(`
			INSERT INTO users (telegram_chat_id, display_name, priority, refresh_token_enc, created_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(telegram_chat_id) DO UPDATE SET
				display_name = excluded.display_name,
				priority = excluded.priority,
				refresh_token_enc = excluded.refresh_token_enc
		`, chatID, nullStr(displayName), priority, enc, now)
		if err != nil {
			return err
		}
	} else {
		if _, err := db.SQL.Exec(`
			INSERT INTO users (telegram_chat_id, display_name, priority, created_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(telegram_chat_id) DO UPDATE SET
				display_name = excluded.display_name,
				priority = excluded.priority
		`, chatID, nullStr(displayName), priority, now); err != nil {
			return err
		}
	}
	_, _ = db.SQL.Exec(`INSERT OR IGNORE INTO user_recurring (telegram_chat_id, templates_json) VALUES (?, '{"templates":[]}')`, chatID)
	return nil
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// UserPriorityOrDefault — текущий приоритет или def для нового пользователя.
func (db *DB) UserPriorityOrDefault(chatID int64, def int) int {
	var p int
	err := db.SQL.QueryRow(`SELECT priority FROM users WHERE telegram_chat_id = ?`, chatID).Scan(&p)
	if err == sql.ErrNoRows {
		return def
	}
	if err != nil {
		return def
	}
	return p
}

// UpdateTelegramUsername — подпись @username из Telegram; только если строка users уже есть.
func (db *DB) UpdateTelegramUsername(chatID int64, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	_, err := db.SQL.Exec(`UPDATE users SET telegram_username = ? WHERE telegram_chat_id = ?`, username, chatID)
	return err
}

// SetMinLeadHours — минимальный запас (часов) до начала занятия; 0 = без ограничения по этому правилу.
func (db *DB) SetMinLeadHours(chatID int64, hours int) error {
	if hours < 0 || hours > 720 {
		return fmt.Errorf("ожидается число часов от 0 до 720")
	}
	res, err := db.SQL.Exec(`UPDATE users SET min_lead_hours = ? WHERE telegram_chat_id = ?`, hours, chatID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("пользователь chat_id=%d не найден", chatID)
	}
	return nil
}

// MinLeadHours — сохранённое значение (для /lead без аргументов).
func (db *DB) MinLeadHours(chatID int64, defaultHours int) (int, error) {
	var h int
	err := db.SQL.QueryRow(`SELECT min_lead_hours FROM users WHERE telegram_chat_id = ?`, chatID).Scan(&h)
	if err == sql.ErrNoRows {
		return defaultHours, nil
	}
	if err != nil {
		return defaultHours, err
	}
	return h, nil
}

// SetPriority — смена приоритета (меньше = раньше в очереди автозаписи).
func (db *DB) SetPriority(chatID int64, priority int) error {
	res, err := db.SQL.Exec(`UPDATE users SET priority = ? WHERE telegram_chat_id = ?`, priority, chatID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("пользователь chat_id=%d не найден", chatID)
	}
	return nil
}

// RefreshToken возвращает расшифрованный refresh-токен ITMO или пустую строку.
func (db *DB) RefreshToken(chatID int64) (string, error) {
	var blob []byte
	err := db.SQL.QueryRow(`SELECT refresh_token_enc FROM users WHERE telegram_chat_id = ?`, chatID).Scan(&blob)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(blob) == 0 {
		return "", nil
	}
	return decryptToken(blob)
}

// HasLinkedITMO — есть сохранённый refresh.
func (db *DB) HasLinkedITMO(chatID int64) (bool, error) {
	tok, err := db.RefreshToken(chatID)
	if err != nil {
		return false, err
	}
	return tok != "", nil
}

// ListUsersWithTokensOrdered — для воркера: приоритет по возрастанию, затем id; только с непустым токеном.
func (db *DB) ListUsersWithTokensOrdered() ([]User, error) {
	rows, err := db.SQL.Query(`
		SELECT id, telegram_chat_id, COALESCE(telegram_username,''), display_name, priority, min_lead_hours, created_at
		FROM users
		WHERE refresh_token_enc IS NOT NULL AND length(refresh_token_enc) > 0
		ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var created sql.NullString
		var dn sql.NullString
		if err := rows.Scan(&u.ID, &u.TelegramChatID, &u.TelegramUsername, &dn, &u.Priority, &u.MinLeadHours, &created); err != nil {
			return nil, err
		}
		if dn.Valid {
			u.DisplayName = dn.String
		}
		if created.Valid {
			u.CreatedAt, _ = time.Parse(time.RFC3339, created.String)
		}
		tok, err := db.RefreshToken(u.TelegramChatID)
		if err != nil || tok == "" {
			continue
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ListAllUsers — для админки (поле Linked).
func (db *DB) ListAllUsers() ([]User, error) {
	rows, err := db.SQL.Query(`
		SELECT id, telegram_chat_id, COALESCE(telegram_username,''), display_name, priority, created_at,
		       CASE WHEN refresh_token_enc IS NOT NULL AND length(refresh_token_enc) > 0 THEN 1 ELSE 0 END
		FROM users ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var created sql.NullString
		var dn sql.NullString
		var linked int
		if err := rows.Scan(&u.ID, &u.TelegramChatID, &u.TelegramUsername, &dn, &u.Priority, &created, &linked); err != nil {
			return nil, err
		}
		if dn.Valid {
			u.DisplayName = dn.String
		}
		if created.Valid {
			u.CreatedAt, _ = time.Parse(time.RFC3339, created.String)
		}
		u.Linked = linked == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

// ImportFromConfig — первый запуск: users из config.json.
func (db *DB) ImportFromConfig(users []config.User) error {
	var n int
	if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, u := range users {
		if u.RefreshToken == "" {
			continue
		}
		prio := u.Priority
		tg := u.TelegramChatID
		if tg == 0 {
			continue
		}
		name := u.Name
		rt := u.RefreshToken
		if err := db.UpsertUser(tg, name, prio, &rt); err != nil {
			return err
		}
	}
	return nil
}

