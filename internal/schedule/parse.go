package schedule

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Occurrence — одно занятие из объединённого JSON (MergeSchedules).
type Occurrence struct {
	LessonID        int64  `json:"id"`
	BuildingID      int64  `json:"building_id"`
	Date            string `json:"date"` // день из корзины API + поля урока
	SectionName     string `json:"section_name"`
	LessonLevelName string `json:"lesson_level_name"`
	TypeName        string `json:"type_name"`
	RoomName        string `json:"room_name"`
	TeacherFIO      string `json:"teacher_fio"`
	TimeSlotStart   string `json:"time_slot_start"`
	TimeSlotEnd     string `json:"time_slot_end"`
	Weekday         int    `json:"-"` // 0=вск … 6=сб, Europe/Moscow
}

// ParseOccurrences разбирает merged JSON (поле result: [{date, lessons}, …]).
func ParseOccurrences(scheduleJSON []byte) ([]Occurrence, error) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.UTC
	}
	var root struct {
		Result []struct {
			Date    string            `json:"date"`
			Lessons []json.RawMessage `json:"lessons"`
		} `json:"result"`
	}
	if err := json.Unmarshal(scheduleJSON, &root); err != nil {
		return nil, err
	}
	var out []Occurrence
	for _, day := range root.Result {
		dayKey := dayBucketDate(day.Date)
		for _, raw := range day.Lessons {
			var o Occurrence
			if err := json.Unmarshal(raw, &o); err != nil {
				return nil, err
			}
			if o.LessonID == 0 {
				continue
			}
			if dayKey != "" {
				o.Date = dayKey
			}
			if wd, e := weekdayMoscow(o.Date, loc); e == nil {
				o.Weekday = wd
			}
			out = append(out, o)
		}
	}
	return out, nil
}

// FindOccurrenceByLessonID ищет занятие по id (первое вхождение).
func FindOccurrenceByLessonID(scheduleJSON []byte, lessonID int64) (*Occurrence, error) {
	all, err := ParseOccurrences(scheduleJSON)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].LessonID == lessonID {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("занятие id=%d не найдено в расписании", lessonID)
}

func dayBucketDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func weekdayMoscow(dateField string, loc *time.Location) (int, error) {
	d := dayBucketDate(dateField)
	if len(d) < 10 {
		return 0, fmt.Errorf("bad date")
	}
	t, err := time.ParseInLocation("2006-01-02", d, loc)
	if err != nil {
		return 0, err
	}
	return int(t.Weekday()), nil
}
