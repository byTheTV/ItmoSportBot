package recurring

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Template — сохранённый «тип» занятия; при появлении нового id на другую дату — запись.
type Template struct {
	ID              string        `json:"id"`
	Fingerprint     Fingerprint   `json:"fingerprint"`
	SignedLessonIDs []int64       `json:"signed_lesson_ids"`
	SourceLessonID  int64         `json:"source_lesson_id,omitempty"`
}

type fileRoot struct {
	Templates []Template `json:"templates"`
}

// FileStore — json рядом с config (путь задаётся снаружи).
type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load() ([]Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var root fileRoot
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	return root.Templates, nil
}

func (s *FileStore) Save(templates []Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := fileRoot{Templates: templates}
	raw, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *FileStore) Add(t Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	list = append(list, t)
	return s.saveUnlocked(list)
}

func (s *FileStore) loadUnlocked() ([]Template, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var root fileRoot
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	return root.Templates, nil
}

func (s *FileStore) saveUnlocked(templates []Template) error {
	root := fileRoot{Templates: templates}
	raw, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// AppendSigned добавляет lesson_id в signed для шаблона с данным id.
func (s *FileStore) AppendSigned(templateID string, lessonID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID != templateID {
			continue
		}
		for _, x := range list[i].SignedLessonIDs {
			if x == lessonID {
				return s.saveUnlocked(list)
			}
		}
		list[i].SignedLessonIDs = append(list[i].SignedLessonIDs, lessonID)
		return s.saveUnlocked(list)
	}
	return fmt.Errorf("шаблон %s не найден", templateID)
}
