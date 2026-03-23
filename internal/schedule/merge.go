package schedule

import (
	"encoding/json"
	"fmt"
	"sort"
)

// BuildingPart — сырой JSON расписания для одного корпуса.
type BuildingPart struct {
	BuildingID int64
	Raw        []byte
}

// MergeSchedules объединяет ответы GET sign/schedule по нескольким building_id в один JSON для FormatDayMessages.
func MergeSchedules(items []BuildingPart) ([]byte, error) {
	if len(items) == 0 {
		return []byte(`{"result":[]}`), nil
	}
	type day struct {
		Date    string            `json:"date"`
		Lessons []json.RawMessage `json:"lessons"`
	}
	byDate := make(map[string][]json.RawMessage)
	seenLesson := make(map[string]map[int64]struct{}) // date -> lesson id

	for _, it := range items {
		var root struct {
			Result []struct {
				Date    string            `json:"date"`
				Lessons []json.RawMessage `json:"lessons"`
			} `json:"result"`
		}
		if err := json.Unmarshal(it.Raw, &root); err != nil {
			return nil, fmt.Errorf("building_id=%d: %w", it.BuildingID, err)
		}
		for _, d := range root.Result {
			if seenLesson[d.Date] == nil {
				seenLesson[d.Date] = make(map[int64]struct{})
			}
			for _, lesson := range d.Lessons {
				var lid struct {
					ID int64 `json:"id"`
				}
				if json.Unmarshal(lesson, &lid) == nil && lid.ID != 0 {
					if _, dup := seenLesson[d.Date][lid.ID]; dup {
						continue
					}
					seenLesson[d.Date][lid.ID] = struct{}{}
				}
				tagged, err := tagBuilding(lesson, it.BuildingID)
				if err != nil {
					return nil, err
				}
				byDate[d.Date] = append(byDate[d.Date], tagged)
			}
		}
	}

	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	out := struct {
		Result []day `json:"result"`
	}{}
	for _, d := range dates {
		out.Result = append(out.Result, day{Date: d, Lessons: byDate[d]})
	}
	return json.Marshal(out)
}

func tagBuilding(lesson json.RawMessage, buildingID int64) (json.RawMessage, error) {
	var m map[string]any
	if err := json.Unmarshal(lesson, &m); err != nil {
		return nil, err
	}
	m["building_id"] = buildingID
	return json.Marshal(m)
}

// MergeLimits объединяет поля result из ответов limits по корпусам.
func MergeLimits(parts [][]byte) ([]byte, error) {
	if len(parts) == 0 {
		return []byte("{}"), nil
	}
	combined := make(map[string]any)
	for _, p := range parts {
		var v map[string]any
		if err := json.Unmarshal(p, &v); err != nil {
			continue
		}
		res, _ := v["result"].(map[string]any)
		for k, val := range res {
			combined[k] = val
		}
	}
	return json.Marshal(map[string]any{"result": combined})
}
