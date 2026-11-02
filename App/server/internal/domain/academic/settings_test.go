package academic

import (
	"fmt"
	"strings"
	"testing"
)

func fullPeriods(configured func(index int) (start, end string, ok bool)) []Period {
	out := make([]Period, 0, DefaultPeriodCount)
	for i := 1; i <= DefaultPeriodCount; i++ {
		p := Period{Index: i}
		if configured != nil {
			if s, e, ok := configured(i); ok {
				ss, ee := s, e
				p.StartTime, p.EndTime, p.Configured = &ss, &ee, true
			}
		}
		out = append(out, p)
	}
	return out
}

func TestDefaultSettingsHasThirteenUnconfiguredPeriods(t *testing.T) {
	s := DefaultSettings("2026S2")
	if s.Revision != 0 || s.PeriodCount != 13 || len(s.Periods) != 13 {
		t.Fatalf("unexpected default: %+v", s)
	}
	for _, p := range s.Periods {
		if p.Configured || p.StartTime != nil || p.EndTime != nil {
			t.Fatalf("period %d should be unconfigured", p.Index)
		}
	}
	if s.WeekAnchor.Source != "UNKNOWN" || s.WeekAnchor.Confirmed || s.WeekAnchor.StartDate != nil {
		t.Fatalf("unexpected anchor: %+v", s.WeekAnchor)
	}
}

func TestNormalizeSettingsAcceptsValidConfiguration(t *testing.T) {
	anchorDate := "2026-08-31"
	in := SettingsInput{
		ExpectedRevision: int64Ptr(0),
		WeekAnchor:       WeekAnchor{StartDate: &anchorDate, Source: "USER", Confirmed: true},
		TotalWeeks:       intPtr(18),
	}
	// Distinct increasing slots: 08:00–08:45, 08:50–09:35, ...
	in.Periods = fullPeriods(func(i int) (string, string, bool) {
		base := 8*60 + (i-1)*50
		return clock(base), clock(base + 45), true
	})
	got, err := NormalizeSettings("2026S2", in)
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	if got.Revision != 0 || got.WeekAnchor.StartDate == nil || *got.WeekAnchor.StartDate != anchorDate {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !got.Periods[0].Configured || *got.Periods[0].StartTime != "08:00" {
		t.Fatalf("period 1 = %+v", got.Periods[0])
	}
}

func clock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

func intPtr(v int) *int { return &v }

func int64Ptr(v int64) *int64 { return &v }

func TestNormalizeSettingsRejectsBadInput(t *testing.T) {
	valid := func() SettingsInput {
		return SettingsInput{ExpectedRevision: int64Ptr(0), Periods: fullPeriods(nil)}
	}
	withPeriods := func(mutate func([]Period) []Period) SettingsInput {
		in := valid()
		in.Periods = mutate(in.Periods)
		return in
	}

	cases := []struct {
		name string
		in   SettingsInput
	}{
		{"not thirteen periods", SettingsInput{ExpectedRevision: int64Ptr(0), Periods: fullPeriods(nil)[:12]}},
		{"missing expected revision", SettingsInput{Periods: fullPeriods(nil)}},
		{"duplicate index", withPeriods(func(p []Period) []Period { p[1].Index = 1; return p })},
		{"missing index", withPeriods(func(p []Period) []Period { p[0].Index = 99; return p })},
		{"start without end", withPeriods(func(p []Period) []Period {
			s := "08:00"
			p[0].StartTime, p[0].Configured = &s, true
			return p
		})},
		{"configured false with times", withPeriods(func(p []Period) []Period {
			s, e := "08:00", "08:45"
			p[0].StartTime, p[0].EndTime = &s, &e
			return p
		})},
		{"start after end", withPeriods(func(p []Period) []Period {
			s, e := "09:00", "08:45"
			p[0].StartTime, p[0].EndTime, p[0].Configured = &s, &e, true
			return p
		})},
		{"overlapping periods", withPeriods(func(p []Period) []Period {
			s1, e1 := "08:00", "09:00"
			s2, e2 := "08:50", "09:40"
			p[0].StartTime, p[0].EndTime, p[0].Configured = &s1, &e1, true
			p[1].StartTime, p[1].EndTime, p[1].Configured = &s2, &e2, true
			return p
		})},
		{"anchor not monday", func() SettingsInput {
			d := "2026-09-01"
			in := valid()
			in.WeekAnchor = WeekAnchor{StartDate: &d, Source: "USER", Confirmed: true}
			return in
		}()},
		{"anchor source not user", func() SettingsInput {
			d := "2026-08-31"
			in := valid()
			in.WeekAnchor = WeekAnchor{StartDate: &d, Source: "CALENDAR", Confirmed: true}
			return in
		}()},
		{"anchor unconfirmed", func() SettingsInput {
			d := "2026-08-31"
			in := valid()
			in.WeekAnchor = WeekAnchor{StartDate: &d, Source: "USER"}
			return in
		}()},
		{"total weeks zero", func() SettingsInput { in := valid(); n := 0; in.TotalWeeks = &n; return in }()},
		{"total weeks too large", func() SettingsInput { in := valid(); n := 61; in.TotalWeeks = &n; return in }()},
		{"period count not 13", func() SettingsInput { in := valid(); n := 12; in.PeriodCount = &n; return in }()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeSettings("2026S2", tc.in); err == nil {
				t.Fatalf("expected an error")
			}
		})
	}
}

func TestValidateSemesterRejectsMagicValue(t *testing.T) {
	if err := ValidateSemester(""); err == nil {
		t.Fatal("empty semester must be rejected")
	}
	if err := ValidateSemester("current"); err == nil {
		t.Fatal("magic value current must be rejected")
	}
	if err := ValidateSemester(strings.Repeat("x", 129)); err == nil {
		t.Fatal("over-long semester must be rejected")
	}
	if err := ValidateSemester("2026S2"); err != nil {
		t.Fatalf("valid semester rejected: %v", err)
	}
}

func TestPeriodTimesRequiresAllPeriodsConfigured(t *testing.T) {
	s := DefaultSettings("2026S2")
	if _, _, ok := s.PeriodTimes([]int{1, 2}); ok {
		t.Fatal("unconfigured periods must not produce times")
	}
	start, end := "08:00", "09:40"
	s.Periods[0].StartTime, s.Periods[0].EndTime, s.Periods[0].Configured = &start, &start, true
	s.Periods[1].StartTime, s.Periods[1].EndTime, s.Periods[1].Configured = &start, &end, true
	if gotStart, gotEnd, ok := s.PeriodTimes([]int{1, 2}); !ok || gotStart != start || gotEnd != end {
		t.Fatalf("times = %q %q %v", gotStart, gotEnd, ok)
	}
	if _, _, ok := s.PeriodTimes([]int{1, 3}); ok {
		t.Fatal("a partially configured range must not produce times")
	}
}
