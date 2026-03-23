package recurring

import (
	"strings"
	"time"
)

// SignOpensAtMSK возвращает момент открытия записи: 00:00 МСК за 14 календарных дней до дня занятия.
func SignOpensAtMSK(lessonDateField string) (time.Time, error) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*3600)
	}
	d := strings.TrimSpace(lessonDateField)
	if len(d) >= 10 {
		d = d[:10]
	}
	t, err := time.ParseInLocation("2006-01-02", d, loc)
	if err != nil {
		return time.Time{}, err
	}
	y, m, day := t.Date()
	lessonMidnight := time.Date(y, m, day, 0, 0, 0, 0, loc)
	return lessonMidnight.AddDate(0, 0, -14), nil
}

// CalendarDateMSK полночь календарного дня в МСК для сравнения «сегодня».
func CalendarDateMSK(t time.Time) time.Time {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*3600)
	}
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
