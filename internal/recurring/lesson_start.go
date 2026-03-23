package recurring

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"itmosportbot/internal/schedule"
)

// MinLeadBeforeLesson — не записываться, если до начала занятия осталось меньше этого интервала.
const MinLeadBeforeLesson = 36 * time.Hour

// LessonStartMSK — дата из слота + time_slot_start в часовом поясе МСК.
func LessonStartMSK(o schedule.Occurrence, loc *time.Location) (time.Time, error) {
	if len(o.Date) < 10 {
		return time.Time{}, fmt.Errorf("пустая дата")
	}
	day, err := time.ParseInLocation("2006-01-02", o.Date[:10], loc)
	if err != nil {
		return time.Time{}, err
	}
	h, m, ok := parseClockHM(o.TimeSlotStart)
	if !ok {
		return time.Time{}, fmt.Errorf("нет time_slot_start")
	}
	y, mo, d := day.Date()
	return time.Date(y, mo, d, h, m, 0, 0, loc), nil
}

func parseClockHM(s string) (hour, min int, ok bool) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return 0, 0, false
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || h < 0 || h > 23 {
		return 0, 0, false
	}
	mm := strings.TrimSpace(parts[1])
	if len(mm) > 2 {
		mm = mm[:2]
	}
	m, err := strconv.Atoi(mm)
	if err != nil || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}
