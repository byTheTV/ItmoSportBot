package recurring

import "itmosportbot/internal/store"

// TemplateFromStore собирает Template из строки БД.
func TemplateFromStore(t store.RecurringTemplate) Template {
	return Template{
		ID:              t.PublicID,
		SourceLessonID:  t.SourceLessonID,
		SignedLessonIDs: append([]int64(nil), t.SignedLessonIDs...),
		Fingerprint: Fingerprint{
			BuildingID:      t.BuildingID,
			Weekday:         t.Weekday,
			TimeSlotStart:   t.TimeSlotStart,
			TimeSlotEnd:     t.TimeSlotEnd,
			SectionName:     t.SectionName,
			RoomName:        t.RoomName,
			TeacherFIO:      t.TeacherFIO,
			TypeName:        t.TypeName,
			LessonLevelName: t.LessonLevelName,
		},
	}
}

// ToStore раскладывает шаблон для INSERT в SQLite.
func (t Template) ToStore() store.RecurringTemplate {
	f := t.Fingerprint
	return store.RecurringTemplate{
		PublicID:        t.ID,
		SourceLessonID:  t.SourceLessonID,
		SignedLessonIDs: append([]int64(nil), t.SignedLessonIDs...),
		BuildingID:      f.BuildingID,
		Weekday:         f.Weekday,
		TimeSlotStart:   f.TimeSlotStart,
		TimeSlotEnd:     f.TimeSlotEnd,
		SectionName:     f.SectionName,
		RoomName:        f.RoomName,
		TeacherFIO:      f.TeacherFIO,
		TypeName:        f.TypeName,
		LessonLevelName: f.LessonLevelName,
	}
}
