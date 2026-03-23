package schedule

import (
	"encoding/json"
	"fmt"
	"slices"
)

// ParseFilterBuildingIDs извлекает id корпусов из JSON ответа GET /api/sport/sign/schedule/filters.
func ParseFilterBuildingIDs(body []byte) ([]int64, error) {
	var root struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	if len(root.Result) == 0 {
		return nil, nil
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(root.Result, &result); err != nil {
		return nil, err
	}
	raw, ok := result["building_id"]
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	var pairs []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &pairs); err == nil && len(pairs) > 0 {
		ids := make([]int64, 0, len(pairs))
		for _, p := range pairs {
			if p.ID > 0 {
				ids = append(ids, p.ID)
			}
		}
		return ids, nil
	}
	var nums []int64
	if err := json.Unmarshal(raw, &nums); err == nil {
		out := nums[:0]
		for _, n := range nums {
			if n > 0 {
				out = append(out, n)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("filters: не удалось разобрать building_id")
}

// UnionBuildingIDs объединяет списки, убирает дубликаты и сортирует.
func UnionBuildingIDs(lists ...[]int64) []int64 {
	seen := make(map[int64]struct{})
	for _, list := range lists {
		for _, id := range list {
			if id > 0 {
				seen[id] = struct{}{}
			}
		}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
