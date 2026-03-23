package recurring

import (
	"strings"

	"itmosportbot/internal/schedule"
)

// Fingerprint совпадает со слотом по полям расписания; lesson_id не сравнивается.
// BuildingID: для шаблонов, созданных после добавления поля — корпус из API; 0 = старые шаблоны (корпус не фиксирован).
type Fingerprint struct {
	BuildingID      int64  `json:"building_id,omitempty"`
	Weekday         int    `json:"weekday"` // time.Weekday: 0=вск … 6=сб
	TimeSlotStart   string `json:"time_slot_start"`
	TimeSlotEnd     string `json:"time_slot_end"`
	SectionName     string `json:"section_name"`
	RoomName        string `json:"room_name"`
	TeacherFIO      string `json:"teacher_fio"`
	TypeName        string `json:"type_name"`
	LessonLevelName string `json:"lesson_level_name"`
}

func FingerprintFromOccurrence(o schedule.Occurrence) Fingerprint {
	return Fingerprint{
		BuildingID:      o.BuildingID,
		Weekday:         o.Weekday,
		TimeSlotStart:   o.TimeSlotStart,
		TimeSlotEnd:     o.TimeSlotEnd,
		SectionName:     o.SectionName,
		RoomName:        o.RoomName,
		TeacherFIO:      o.TeacherFIO,
		TypeName:        o.TypeName,
		LessonLevelName: o.LessonLevelName,
	}
}

func (f Fingerprint) Matches(o schedule.Occurrence) bool {
	if f.BuildingID != 0 && o.BuildingID != f.BuildingID {
		return false
	}
	if f.Weekday != o.Weekday {
		return false
	}
	if normTime(f.TimeSlotStart) != normTime(o.TimeSlotStart) {
		return false
	}
	if normTime(f.TimeSlotEnd) != normTime(o.TimeSlotEnd) {
		return false
	}
	if normSpace(f.SectionName) != normSpace(o.SectionName) {
		return false
	}
	if normSpace(f.RoomName) != normSpace(o.RoomName) {
		return false
	}
	if normSpace(f.TeacherFIO) != normSpace(o.TeacherFIO) {
		return false
	}
	if normSpace(f.TypeName) != normSpace(o.TypeName) {
		return false
	}
	if normSpace(f.LessonLevelName) != normSpace(o.LessonLevelName) {
		return false
	}
	return true
}

// BuildingIDsForSign список building_id для POST записи: один корпус из шаблона, если задан; иначе полный union (как раньше).
func BuildingIDsForSign(fp Fingerprint, union []int64) []int64 {
	if fp.BuildingID > 0 {
		return []int64{fp.BuildingID}
	}
	return union
}

func normSpace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func normTime(s string) string {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return s
	}
	hh := strings.TrimSpace(parts[0])
	mm := strings.TrimSpace(parts[1])
	if len(mm) > 2 {
		mm = mm[:2]
	}
	if hh == "" || mm == "" {
		return s
	}
	return hh + ":" + mm
}
