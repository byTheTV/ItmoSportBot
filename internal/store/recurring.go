package store

import (
	"database/sql"
	"fmt"
)

// RecurringTemplate — шаблон автозаписи в таблицах SQLite (без зависимости от пакета recurring).
type RecurringTemplate struct {
	PublicID        string
	SourceLessonID  int64
	SignedLessonIDs []int64
	BuildingID      int64
	Weekday         int
	TimeSlotStart   string
	TimeSlotEnd     string
	SectionName     string
	RoomName        string
	TeacherFIO      string
	TypeName        string
	LessonLevelName string
}

// ListRecurringTemplates — порядок как в /list (ORDER BY id).
func (db *DB) ListRecurringTemplates(chatID int64) ([]RecurringTemplate, error) {
	rows, err := db.SQL.Query(`
		SELECT id, public_id, source_lesson_id, building_id, weekday,
		       time_slot_start, time_slot_end, section_name, room_name,
		       teacher_fio, type_name, lesson_level_name
		FROM recurring_templates
		WHERE telegram_chat_id = ?
		ORDER BY id ASC
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecurringTemplate
	var rowIDs []int64
	for rows.Next() {
		var rowID int64
		var t RecurringTemplate
		err := rows.Scan(
			&rowID, &t.PublicID, &t.SourceLessonID, &t.BuildingID, &t.Weekday,
			&t.TimeSlotStart, &t.TimeSlotEnd, &t.SectionName, &t.RoomName,
			&t.TeacherFIO, &t.TypeName, &t.LessonLevelName,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		rowIDs = append(rowIDs, rowID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(rowIDs) == 0 {
		return out, nil
	}
	signed, err := db.signedLessonsForTemplates(rowIDs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].SignedLessonIDs = signed[rowIDs[i]]
	}
	return out, nil
}

func (db *DB) signedLessonsForTemplates(rowIDs []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(rowIDs))
	if len(rowIDs) == 0 {
		return out, nil
	}
	// SQLite: build placeholders
	placeholders := ""
	args := make([]interface{}, 0, len(rowIDs))
	for i, id := range rowIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}
	q := `SELECT template_row_id, lesson_id FROM recurring_signed WHERE template_row_id IN (` + placeholders + `) ORDER BY lesson_id ASC`
	rows, err := db.SQL.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tid, lid int64
		if err := rows.Scan(&tid, &lid); err != nil {
			return nil, err
		}
		out[tid] = append(out[tid], lid)
	}
	return out, rows.Err()
}

// InsertRecurringTemplate добавляет шаблон и опционально стартовые signed id (импорт).
func (db *DB) InsertRecurringTemplate(chatID int64, t RecurringTemplate) error {
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`
		INSERT INTO recurring_templates (
			telegram_chat_id, public_id, source_lesson_id, building_id, weekday,
			time_slot_start, time_slot_end, section_name, room_name,
			teacher_fio, type_name, lesson_level_name
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
	`, chatID, t.PublicID, t.SourceLessonID, t.BuildingID, t.Weekday,
		t.TimeSlotStart, t.TimeSlotEnd, t.SectionName, t.RoomName,
		t.TeacherFIO, t.TypeName, t.LessonLevelName)
	if err != nil {
		return err
	}
	rowID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for _, lid := range t.SignedLessonIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO recurring_signed (template_row_id, lesson_id) VALUES (?,?)`, rowID, lid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteRecurringTemplateByOneBasedIndex удаляет шаблон с номером как в /list (1-based).
func (db *DB) DeleteRecurringTemplateByOneBasedIndex(chatID int64, n int) error {
	if n < 1 {
		return fmt.Errorf("некорректный номер")
	}
	list, err := db.ListRecurringTemplates(chatID)
	if err != nil {
		return err
	}
	if n > len(list) {
		return fmt.Errorf("нет шаблона #%d", n)
	}
	pid := list[n-1].PublicID
	_, err = db.SQL.Exec(`
		DELETE FROM recurring_templates WHERE telegram_chat_id = ? AND public_id = ?
	`, chatID, pid)
	return err
}

// AppendSignedLesson добавляет lesson_id к шаблону public_id (идемпотентно).
func (db *DB) AppendSignedLesson(chatID int64, publicID string, lessonID int64) error {
	var rowID int64
	err := db.SQL.QueryRow(`
		SELECT id FROM recurring_templates WHERE telegram_chat_id = ? AND public_id = ?
	`, chatID, publicID).Scan(&rowID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("шаблон %s не найден", publicID)
	}
	if err != nil {
		return err
	}
	_, err = db.SQL.Exec(`INSERT OR IGNORE INTO recurring_signed (template_row_id, lesson_id) VALUES (?,?)`, rowID, lessonID)
	return err
}

// CountRecurringTemplates число шаблонов пользователя.
func (db *DB) CountRecurringTemplates(chatID int64) (int, error) {
	var n int
	err := db.SQL.QueryRow(`SELECT COUNT(*) FROM recurring_templates WHERE telegram_chat_id = ?`, chatID).Scan(&n)
	return n, err
}
