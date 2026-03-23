package buildings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type file struct {
	BuildingIDs []int64 `json:"building_ids"`
}

// Load читает JSON вида {"building_ids":[273,...]}.
func Load(path string) ([]int64, error) {
	path = filepath.Clean(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := make([]int64, 0, len(f.BuildingIDs))
	for _, id := range f.BuildingIDs {
		if id > 0 {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: пустой или нулевой building_ids", path)
	}
	return out, nil
}
