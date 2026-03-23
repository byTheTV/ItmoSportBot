package store

import (
	"encoding/json"
	"os"
)

// ImportRecurringFile — legacy файл recurring_templates.json → SQL, если у пользователя ещё нет строк в recurring_templates.
func (db *DB) ImportRecurringFile(path string, chatID int64) error {
	var n int
	if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM recurring_templates WHERE telegram_chat_id = ?`, chatID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var root jsonFileRoot
	if err := json.Unmarshal(raw, &root); err != nil {
		return err
	}
	if len(root.Templates) == 0 {
		return nil
	}
	return applyJSONRoot(db, chatID, &root)
}
