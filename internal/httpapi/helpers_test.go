package httpapi

import (
	"testing"
	"time"
)

func TestNormalizeDays(t *testing.T) {
	t.Run("valid and unique", func(t *testing.T) {
		got, ok := normalizeDays([]int{1, 3, 3, 7})
		if !ok {
			t.Fatal("expected ok")
		}
		if len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 7 {
			t.Fatalf("unexpected days: %#v", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, ok := normalizeDays([]int{0})
		if ok {
			t.Fatal("expected not ok")
		}
	})
}

func TestIsValidTimeRange(t *testing.T) {
	if !isValidTimeRange("09:00", "10:30") {
		t.Fatal("expected valid")
	}
	if isValidTimeRange("10:30", "09:00") {
		t.Fatal("expected invalid")
	}
	if isValidTimeRange("ab:cd", "09:00") {
		t.Fatal("expected invalid")
	}
}

func TestGenerateDailyIntervals(t *testing.T) {
	date := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	intervals, ok := generateDailyIntervals(date, "09:00", "11:00")
	if !ok {
		t.Fatal("expected ok")
	}
	if len(intervals) != 4 {
		t.Fatalf("expected 4 intervals, got %d", len(intervals))
	}
	if intervals[0][0].Format(time.RFC3339) != "2026-04-03T09:00:00Z" {
		t.Fatalf("unexpected first interval start: %s", intervals[0][0])
	}
	if intervals[3][1].Format(time.RFC3339) != "2026-04-03T11:00:00Z" {
		t.Fatalf("unexpected last interval end: %s", intervals[3][1])
	}
}

func TestParsePagination(t *testing.T) {
	page, size, ok := parsePagination("", "")
	if !ok || page != 1 || size != 20 {
		t.Fatalf("unexpected defaults: %d %d %v", page, size, ok)
	}

	page, size, ok = parsePagination("2", "50")
	if !ok || page != 2 || size != 50 {
		t.Fatalf("unexpected values: %d %d %v", page, size, ok)
	}

	_, _, ok = parsePagination("0", "10")
	if ok {
		t.Fatal("expected invalid page")
	}
	_, _, ok = parsePagination("1", "101")
	if ok {
		t.Fatal("expected invalid pageSize")
	}
}

func TestDayOfWeek(t *testing.T) {
	monday := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	if dayOfWeek(monday) != 1 {
		t.Fatalf("expected monday=1, got %d", dayOfWeek(monday))
	}
	if dayOfWeek(sunday) != 7 {
		t.Fatalf("expected sunday=7, got %d", dayOfWeek(sunday))
	}
}
