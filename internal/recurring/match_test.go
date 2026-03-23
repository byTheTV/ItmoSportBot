package recurring

import (
	"testing"

	"itmosportbot/internal/schedule"
)

func TestBuildingIDsForSign(t *testing.T) {
	union := []int64{5, 273, 483}
	if got := BuildingIDsForSign(Fingerprint{BuildingID: 273}, union); len(got) != 1 || got[0] != 273 {
		t.Fatalf("got %v", got)
	}
	if got := BuildingIDsForSign(Fingerprint{}, union); len(got) != 3 {
		t.Fatalf("legacy: got %v", got)
	}
}

func TestFingerprintMatches_building(t *testing.T) {
	base := schedule.Occurrence{
		BuildingID:      273,
		Weekday:         1,
		TimeSlotStart:   "10:00",
		TimeSlotEnd:     "11:30",
		SectionName:     "Sec",
		RoomName:        "R",
		TeacherFIO:      "Ivanov I.I.",
		TypeName:        "T",
		LessonLevelName: "L",
	}
	f := FingerprintFromOccurrence(base)
	other := base
	other.BuildingID = 484
	if !f.Matches(base) {
		t.Fatal("same building")
	}
	if f.Matches(other) {
		t.Fatal("different building")
	}
}
