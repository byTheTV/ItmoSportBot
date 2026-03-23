package recurring

import (
	"testing"
	"time"
)

func TestInFastPollWindow_wrapsMidnight(t *testing.T) {
	loc := mskLoc()
	// 2025-03-23 23:55 MSK
	d1 := time.Date(2025, 3, 23, 23, 55, 0, 0, loc)
	if !InFastPollWindow(d1, "23:50", "00:15") {
		t.Fatal("23:55 should be in window")
	}
	// 00:10 same calendar day
	d2 := time.Date(2025, 3, 24, 0, 10, 0, 0, loc)
	if !InFastPollWindow(d2, "23:50", "00:15") {
		t.Fatal("00:10 should be in window")
	}
	// 12:00
	d3 := time.Date(2025, 3, 24, 12, 0, 0, 0, loc)
	if InFastPollWindow(d3, "23:50", "00:15") {
		t.Fatal("12:00 should be outside window")
	}
}
