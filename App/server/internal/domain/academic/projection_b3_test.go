package academic

import (
	"testing"

	"kairos/server/internal/domain/calendar"
)

func settingsForProjection() Settings {
	periods := make([]Period, 13)
	for i := 1; i <= 13; i++ {
		p := Period{Index: i}
		switch i {
		case 12:
			p.StartTime, p.EndTime, p.Configured = strPtrP("19:00"), strPtrP("19:45"), true
		case 13:
			p.StartTime, p.EndTime, p.Configured = strPtrP("19:55"), strPtrP("20:40"), true
		default:
			p.StartTime, p.EndTime, p.Configured = strPtrP("08:00"), strPtrP("08:45"), true
		}
		periods[i-1] = p
	}
	return Settings{
		SchemaVersion: 1, Semester: "2026S2", Revision: 1, TimeZone: "Asia/Shanghai",
		WeekAnchor:   WeekAnchor{StartDate: strPtrP("2026-08-31"), Source: "USER", Confirmed: true},
		WeekStartDay: 1, PeriodCount: 13, Periods: periods,
	}
}

func strPtrP(s string) *string { return &s }

// TestProjectionLatePeriodsAndOddWeeks covers 12/13th periods, odd weeks and
// non-contiguous sections.
func TestProjectionLatePeriodsAndOddWeeks(t *testing.T) {
	settings := settingsForProjection()
	source := Values{Weekday: 4, Sections: []int{12, 13}, Weeks: []int{1, 3, 5}}
	effective := source
	proj, err := BuildProjection(source, effective, settings)
	if err != nil {
		t.Fatalf("projection: %v", err)
	}
	if proj.StartTime != "19:00" || proj.EndTime != "20:40" {
		t.Fatalf("times = %s-%s", proj.StartTime, proj.EndTime)
	}
	if proj.WeekPattern != "1周,3周,5周" {
		t.Fatalf("week pattern = %q", proj.WeekPattern)
	}
	if proj.DayOfWeek != 4 {
		t.Fatalf("day = %d", proj.DayOfWeek)
	}
	if proj.SemesterStartDate != "2026-08-31" {
		t.Fatalf("anchor = %q", proj.SemesterStartDate)
	}
}

// TestProjectionRejectsUnconfiguredPeriod pins the "unknown times" rule.
func TestProjectionRejectsUnconfiguredPeriod(t *testing.T) {
	settings := settingsForProjection()
	settings.Periods[11].Configured = false
	settings.Periods[11].StartTime = nil
	source := Values{Weekday: 1, Sections: []int{12}, Weeks: []int{1}}
	if _, err := BuildProjection(source, source, settings); err == nil {
		t.Fatal("expected TIMES_REQUIRED")
	}
}

// TestWeekPatternMatchesCalendar verifies the generated pattern survives the
// legacy matcher for all 1..60 weeks (mixed odd/even behaviour).
func TestWeekPatternMatchesCalendar(t *testing.T) {
	weeks := []int{1, 2, 3, 5, 8, 13, 21}
	pattern, ok := WeekPatternFromWeeks(weeks)
	if !ok {
		t.Fatal("pattern not representable")
	}
	for w := 1; w <= 60; w++ {
		want := false
		for _, x := range weeks {
			if x == w {
				want = true
			}
		}
		got := calendar.MatchesWeekPattern(pattern, int64(w))
		if got != want {
			t.Fatalf("week %d: pattern %q -> %v, want %v", w, pattern, got, want)
		}
	}
}
