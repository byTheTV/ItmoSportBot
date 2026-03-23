package recurring

import (
	"fmt"
	"strings"
	"time"
)

func mskLoc() *time.Location {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return time.FixedZone("MSK", 3*3600)
	}
	return loc
}

// parseClockMinutes разбирает "HH:MM" (МСК, только часы:минуты) в минуты от полуночи [0,1439].
func parseClockMinutes(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("пусто")
	}
	var h, m int
	n, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil || n != 2 {
		return 0, fmt.Errorf("нужен формат HH:MM")
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("вне диапазона")
	}
	return h*60 + m, nil
}

// InFastPollWindow — true, если текущее время по МСК попадает в [start, end], end может быть на следующий день (например 23:50–00:15).
func InFastPollWindow(now time.Time, startHHMM, endHHMM string) bool {
	t := now.In(mskLoc())
	sm, err1 := parseClockMinutes(startHHMM)
	em, err2 := parseClockMinutes(endHHMM)
	if err1 != nil || err2 != nil {
		return false
	}
	cur := t.Hour()*60 + t.Minute()
	if sm > em {
		return cur >= sm || cur <= em
	}
	return cur >= sm && cur <= em
}

// NextPollInterval — интервал до следующего тика: fast в окне полуночи, иначе slow.
func NextPollInterval(now time.Time, fast, slow time.Duration, startHHMM, endHHMM string) time.Duration {
	if InFastPollWindow(now, startHHMM, endHHMM) {
		return fast
	}
	return slow
}
