package recurring

import (
	"crypto/rand"
	"encoding/hex"

	"itmosportbot/internal/schedule"
)

// NewTemplate строит шаблон по найденному занятию (id только для справки в файле).
func NewTemplate(sourceLessonID int64, o schedule.Occurrence) Template {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return Template{
		ID:              hex.EncodeToString(b),
		Fingerprint:     FingerprintFromOccurrence(o),
		SourceLessonID:  sourceLessonID,
		SignedLessonIDs: nil,
	}
}
