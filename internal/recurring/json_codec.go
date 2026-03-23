package recurring

import (
	"encoding/json"
	"fmt"
)

// EncodeTemplatesFileJSON — тот же формат, что и recurring_templates.json.
func EncodeTemplatesFileJSON(templates []Template) ([]byte, error) {
	root := fileRoot{Templates: templates}
	return json.MarshalIndent(root, "", "  ")
}

// DecodeTemplatesFileJSON разбирает полный файл или пустой слайс.
func DecodeTemplatesFileJSON(raw []byte) ([]Template, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var root fileRoot
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	return root.Templates, nil
}

// AppendSignedToJSON обновляет JSON файла; blob может быть nil/пустым.
func AppendSignedToJSON(blob []byte, templateID string, lessonID int64) ([]byte, error) {
	list, err := DecodeTemplatesFileJSON(blob)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID != templateID {
			continue
		}
		for _, x := range list[i].SignedLessonIDs {
			if x == lessonID {
				return EncodeTemplatesFileJSON(list)
			}
		}
		list[i].SignedLessonIDs = append(list[i].SignedLessonIDs, lessonID)
		return EncodeTemplatesFileJSON(list)
	}
	return nil, fmt.Errorf("шаблон %s не найден", templateID)
}
