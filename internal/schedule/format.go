package schedule

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxMsgRunes = 3500 // запас под лимит Telegram

func joinBuildingIDs(ids []int64) string {
	if len(ids) == 0 {
		return "—"
	}
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	return b.String()
}

// FormatDayMessages — заголовок (все корпуса из config) + блоки по корпусам со слотами.
// loadFailed — building_id, по которым API вернул ошибку (не путать с «на дату пусто»).
// mergedFilterBuildings — если true, в опрос входили все building_id из GET sign/schedule/filters (union с config); totalPolled — число уникальных id.
func FormatDayMessages(date string, scheduleJSON, limitsJSON []byte, requestedBuildings, loadFailed []int64, mergedFilterBuildings bool, totalPolled int) ([]string, error) {
	var sched struct {
		Result []struct {
			Date    string      `json:"date"`
			Lessons []lessonRow `json:"lessons"`
		} `json:"result"`
	}
	if err := json.Unmarshal(scheduleJSON, &sched); err != nil {
		return nil, err
	}
	var limitsRoot map[string]any
	_ = json.Unmarshal(limitsJSON, &limitsRoot)
	limitsResult, _ := limitsRoot["result"].(map[string]any)

	var lessons []lessonRow
	for _, day := range sched.Result {
		if day.Date != date {
			continue
		}
		lessons = append(lessons, day.Lessons...)
	}
	if len(lessons) == 0 {
		line := fmt.Sprintf("На %s занятий нет.", date)
		if len(requestedBuildings) > 0 {
			line += fmt.Sprintf("\nЗапрошены корпуса: %s", joinBuildingIDs(requestedBuildings))
		} else {
			line += " Проверь дату и building_ids в config."
		}
		if len(loadFailed) > 0 {
			line += fmt.Sprintf("\nОшибка API при загрузке: %s", joinBuildingIDs(loadFailed))
		}
		return []string{line}, nil
	}

	byB := groupByBuilding(lessons)
	bids := sortedBuildingKeys(byB)

	failedSet := make(map[int64]struct{}, len(loadFailed))
	for _, id := range loadFailed {
		failedSet[id] = struct{}{}
	}
	var emptyOnDate []int64
	seen := make(map[int64]struct{}, len(bids))
	for _, b := range bids {
		seen[b] = struct{}{}
	}
	for _, id := range requestedBuildings {
		if _, bad := failedSet[id]; bad {
			continue
		}
		if _, ok := seen[id]; !ok {
			emptyOnDate = append(emptyOnDate, id)
		}
	}

	var head strings.Builder
	fmt.Fprintf(&head, "📅 %s", date)
	if len(requestedBuildings) > 0 {
		fmt.Fprintf(&head, "\nКорпуса из config (%d): %s", len(requestedBuildings), joinBuildingIDs(requestedBuildings))
	}
	if mergedFilterBuildings && totalPolled > 0 {
		fmt.Fprintf(&head, "\nОпрос расписания: %d уникальных building_id (config ∪ API filters)", totalPolled)
	}
	if len(loadFailed) > 0 {
		fmt.Fprintf(&head, "\nНе загрузилось (ошибка API): %s", joinBuildingIDs(loadFailed))
	}
	if len(emptyOnDate) > 0 {
		fmt.Fprintf(&head, "\nНа эту дату пар нет (пустой ответ): %s", joinBuildingIDs(emptyOnDate))
	}
	fmt.Fprintf(&head, "\nЕсть пары на дату — корпусов %d: %s", len(bids), joinBuildingIDs(bids))

	msgs := make([]string, 0, len(bids)+2)
	msgs = append(msgs, head.String())

	for _, bid := range bids {
		ls := byB[bid]
		sortLessonsByTime(ls)
		chunks := splitLessonsToChunks(bid, ls, limitsResult, maxMsgRunes)
		msgs = append(msgs, chunks...)
	}
	return msgs, nil
}

func groupByBuilding(lessons []lessonRow) map[int64][]lessonRow {
	m := make(map[int64][]lessonRow)
	for _, l := range lessons {
		bid := l.BuildingID
		m[bid] = append(m[bid], l)
	}
	return m
}

func sortedBuildingKeys(m map[int64][]lessonRow) []int64 {
	keys := make([]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, aj := keys[i], keys[j]
		if ai == 0 && aj != 0 {
			return false
		}
		if aj == 0 && ai != 0 {
			return true
		}
		return ai < aj
	})
	return keys
}

func sortLessonsByTime(ls []lessonRow) {
	sort.Slice(ls, func(i, j int) bool {
		return lessonTimeSortKey(ls[i]) < lessonTimeSortKey(ls[j])
	})
}

func lessonTimeSortKey(l lessonRow) string {
	t0, _ := lessonTimes(l)
	if len(t0) >= 4 {
		return t0
	}
	return "99:99"
}

func splitLessonsToChunks(bid int64, ls []lessonRow, limitsResult map[string]any, maxRunes int) []string {
	var chunks []string
	part := 0
	remaining := ls

	for len(remaining) > 0 {
		head := buildingTitle(bid)
		if part > 0 {
			head = fmt.Sprintf("%s · продолжение", buildingTitle(bid))
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n%s\n\n", head, "────────────────")
		used := utf8.RuneCountInString(b.String())
		first := true
		n := 0
		for n < len(remaining) {
			block := formatLessonBlock(remaining[n], limitsResult)
			sep := "\n\n"
			if first {
				sep = ""
				first = false
			}
			add := sep + block
			if used+utf8.RuneCountInString(add) > maxRunes && n > 0 {
				break
			}
			b.WriteString(add)
			used += utf8.RuneCountInString(add)
			n++
		}
		if n == 0 {
			block := formatLessonBlock(remaining[0], limitsResult)
			b.WriteString(block)
			n = 1
		}
		chunks = append(chunks, strings.TrimSpace(b.String()))
		remaining = remaining[n:]
		part++
	}
	return chunks
}

func buildingTitle(bid int64) string {
	if bid == 0 {
		return "🏢 Корпус не указан"
	}
	return fmt.Sprintf("🏢 Здание %d", bid)
}

func formatLessonBlock(l lessonRow, limitsResult map[string]any) string {
	t0, t1 := lessonTimes(l)
	teacher := shortName(l.TeacherFIO)
	section := strings.TrimSpace(l.SectionName)
	kind := strings.TrimSpace(l.LessonLevelName)
	if kind == "" {
		kind = strings.TrimSpace(l.TypeName)
	}
	room := strings.TrimSpace(l.RoomName)
	if room != "" {
		room = truncateRunes(room, 72)
	}
	spots := spotsShort(l, limitsResult)

	var lines []string
	head := fmt.Sprintf("• %s–%s  %s", t0, t1, section)
	if kind != "" && kind != section {
		head = fmt.Sprintf("• %s–%s  %s · %s", t0, t1, section, truncateRunes(kind, 36))
	}
	lines = append(lines, head)
	lines = append(lines, fmt.Sprintf("  id %d  ·  %s", l.ID, spots))
	if room != "" {
		lines = append(lines, fmt.Sprintf("  %s", room))
	}
	if teacher != "" {
		lines = append(lines, fmt.Sprintf("  👤 %s", teacher))
	}
	return strings.Join(lines, "\n")
}

func spotsShort(l lessonRow, limitsResult map[string]any) string {
	if l.Limit != nil && l.Available != nil {
		return fmt.Sprintf("%g/%g мест", *l.Available, *l.Limit)
	}
	s := lessonLimit(l, limitsResult)
	if s == "н/д" {
		return "мест н/д"
	}
	const mark = " (доступно: "
	if idx := strings.Index(s, mark); idx >= 0 {
		limit := strings.TrimSpace(s[:idx])
		avail := strings.TrimSuffix(strings.TrimSpace(s[idx+len(mark):]), ")")
		return fmt.Sprintf("%s/%s мест", avail, limit)
	}
	return s
}

func shortName(fio string) string {
	fio = strings.TrimSpace(fio)
	if fio == "" {
		return ""
	}
	if i := strings.IndexByte(fio, ' '); i > 0 {
		return fio[:i]
	}
	return fio
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

type lessonRow struct {
	ID              int64    `json:"id"`
	BuildingID      int64    `json:"building_id"`
	SectionName     string   `json:"section_name"`
	LessonLevelName string   `json:"lesson_level_name"`
	TypeName        string   `json:"type_name"`
	Date            string   `json:"date"`
	DateEnd         string   `json:"date_end"`
	RoomName        string   `json:"room_name"`
	TeacherFIO      string   `json:"teacher_fio"`
	Limit           *float64 `json:"limit"`
	Available       *float64 `json:"available"`
	TimeSlotStart   string   `json:"time_slot_start"`
	TimeSlotEnd     string   `json:"time_slot_end"`
}

func lessonLimit(l lessonRow, limitsResult map[string]any) string {
	if l.Limit != nil && l.Available != nil {
		return fmt.Sprintf("%g (доступно: %g)", *l.Limit, *l.Available)
	}
	return lookupLimit(limitsResult, l.ID)
}

func lessonTimes(l lessonRow) (string, string) {
	if strings.TrimSpace(l.TimeSlotStart) != "" && strings.TrimSpace(l.TimeSlotEnd) != "" {
		return l.TimeSlotStart, l.TimeSlotEnd
	}
	return formatTimePair(l.Date, l.DateEnd)
}

func lookupLimit(limitsResult map[string]any, lessonID int64) string {
	idStr := fmt.Sprintf("%d", lessonID)
	if limitsResult == nil {
		return "н/д"
	}
	for _, v := range limitsResult {
		sec, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if info, ok := sec[idStr].(map[string]any); ok {
			limit, _ := info["limit"]
			avail, _ := info["available"]
			return fmt.Sprintf("%v (доступно: %v)", limit, avail)
		}
	}
	return "н/д"
}

func formatTimePair(startISO, endISO string) (string, string) {
	t0, e0 := parseAnyTime(startISO)
	t1, e1 := parseAnyTime(endISO)
	if e0 != nil || e1 != nil {
		return "?", "?"
	}
	return t0.Format("15:04"), t1.Format("15:04")
}

func parseAnyTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05+0700",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("time: %q", s)
}
