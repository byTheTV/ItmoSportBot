package store

import (
	"database/sql"
	"encoding/json"
	"log"
)

func applyJSONRoot(db *DB, chatID int64, root *jsonFileRoot) error {
	for _, jt := range root.Templates {
		t := RecurringTemplate{
			PublicID:        jt.ID,
			SourceLessonID:  jt.SourceLessonID,
			SignedLessonIDs: append([]int64(nil), jt.SignedLessonIDs...),
			BuildingID:      jt.Fingerprint.BuildingID,
			Weekday:         jt.Fingerprint.Weekday,
			TimeSlotStart:   jt.Fingerprint.TimeSlotStart,
			TimeSlotEnd:     jt.Fingerprint.TimeSlotEnd,
			SectionName:     jt.Fingerprint.SectionName,
			RoomName:        jt.Fingerprint.RoomName,
			TeacherFIO:      jt.Fingerprint.TeacherFIO,
			TypeName:        jt.Fingerprint.TypeName,
			LessonLevelName: jt.Fingerprint.LessonLevelName,
		}
		if err := db.InsertRecurringTemplate(chatID, t); err != nil {
			return err
		}
	}
	_, err := db.SQL.Exec(`UPDATE user_recurring SET templates_json = '{"templates":[]}' WHERE telegram_chat_id = ?`, chatID)
	return err
}

// jsonFileRoot дублирует формат recurring JSON для одноразовой миграции (без импорта recurring).
type jsonFileRoot struct {
	Templates []jsonTemplate `json:"templates"`
}

type jsonTemplate struct {
	ID              string          `json:"id"`
	SourceLessonID  int64           `json:"source_lesson_id"`
	SignedLessonIDs []int64         `json:"signed_lesson_ids"`
	Fingerprint     jsonFingerprint `json:"fingerprint"`
}

type jsonFingerprint struct {
	BuildingID      int64  `json:"building_id"`
	Weekday         int    `json:"weekday"`
	TimeSlotStart   string `json:"time_slot_start"`
	TimeSlotEnd     string `json:"time_slot_end"`
	SectionName     string `json:"section_name"`
	RoomName        string `json:"room_name"`
	TeacherFIO      string `json:"teacher_fio"`
	TypeName        string `json:"type_name"`
	LessonLevelName string `json:"lesson_level_name"`
}

func migrateRecurringJSONToTables(db *DB) error {
	rows, err := db.SQL.Query(`SELECT telegram_chat_id, templates_json FROM user_recurring`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var chatID int64
		var js sql.NullString
		if err := rows.Scan(&chatID, &js); err != nil {
			return err
		}
		if !js.Valid || len(js.String) < 15 {
			continue
		}
		var n int
		if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM recurring_templates WHERE telegram_chat_id = ?`, chatID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		var root jsonFileRoot
		if err := json.Unmarshal([]byte(js.String), &root); err != nil {
			log.Printf("migrate templates chat=%d: %v", chatID, err)
			continue
		}
		if len(root.Templates) == 0 {
			continue
		}
		if err := applyJSONRoot(db, chatID, &root); err != nil {
			log.Printf("migrate apply chat=%d: %v", chatID, err)
			continue
		}
		log.Printf("migrate: шаблоны chat_id=%d перенесены в SQL (%d шт.)", chatID, len(root.Templates))
	}
	return rows.Err()
}
