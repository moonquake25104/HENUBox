// Package academic holds the HENU teaching-schedule domain: semester settings
// (periods and week anchor) and the preview built from a parsed export.
// It has no HTTP or database dependencies so it can be unit tested directly.
package academic

import (
	"fmt"
	"strings"
	"time"
)

// DefaultPeriodCount is the confirmed number of periods per day for HENU v1.
const DefaultPeriodCount = 13

// TimeZone is fixed for the whole academic domain.
const TimeZone = "Asia/Shanghai"

// Period is one period of the daily timetable. Unknown times stay null; a
// period is "configured" only when both times are valid.
type Period struct {
	Index      int     `json:"index"`
	StartTime  *string `json:"start_time"`
	EndTime    *string `json:"end_time"`
	Configured bool    `json:"configured"`
}

// WeekAnchor is the first teaching week. An unconfirmed anchor never derives
// actual dates.
type WeekAnchor struct {
	StartDate *string `json:"start_date"`
	Source    string  `json:"source"`
	Confirmed bool    `json:"confirmed"`
}

// Settings is the full semester configuration returned by GET.
type Settings struct {
	SchemaVersion int        `json:"schema_version"`
	Semester      string     `json:"semester"`
	Revision      int64      `json:"revision"`
	TimeZone      string     `json:"time_zone"`
	SemesterLabel *string    `json:"semester_label"`
	WeekAnchor    WeekAnchor `json:"week_anchor"`
	WeekStartDay  int        `json:"week_start_day"`
	TotalWeeks    *int       `json:"total_weeks"`
	PeriodCount   int        `json:"period_count"`
	Periods       []Period   `json:"periods"`
}

// SettingsInput is the PUT body (without schema_version/revision, which the
// server owns) plus the optimistic-concurrency token.
type SettingsInput struct {
	Semester         string     `json:"semester"`
	ExpectedRevision *int64     `json:"expected_revision"`
	SemesterLabel    *string    `json:"semester_label"`
	WeekAnchor       WeekAnchor `json:"week_anchor"`
	TotalWeeks       *int       `json:"total_weeks"`
	PeriodCount      *int       `json:"period_count"`
	Periods          []Period   `json:"periods"`
}

// DefaultSettings returns the unpersisted default for a semester (revision 0).
func DefaultSettings(semester string) Settings {
	periods := make([]Period, 0, DefaultPeriodCount)
	for i := 1; i <= DefaultPeriodCount; i++ {
		periods = append(periods, Period{Index: i})
	}
	return Settings{
		SchemaVersion: 1,
		Semester:      semester,
		Revision:      0,
		TimeZone:      TimeZone,
		WeekAnchor:    WeekAnchor{Source: "UNKNOWN"},
		WeekStartDay:  1,
		PeriodCount:   DefaultPeriodCount,
		Periods:       periods,
	}
}

// ValidationError carries the API error code for a rejected input.
type ValidationError struct {
	Code    string
	Message string
	Details any
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) *ValidationError {
	return &ValidationError{Code: "INVALID_ARGUMENT", Message: fmt.Sprintf(format, args...)}
}

// ValidateSemester guards the opaque semester identifier used by the legacy
// courses filter. It is not a numeric semester id and not a school label.
func ValidateSemester(semester string) error {
	if semester == "" {
		return invalid("semester 不能为空")
	}
	if semester == "current" {
		return invalid("semester 不支持魔法值 current")
	}
	if len([]rune(semester)) > 128 {
		return invalid("semester 长度不能超过 128")
	}
	return nil
}

// NormalizeSettings validates a PUT body and returns the settings to persist.
// The caller supplies the semester and the revision it will store.
func NormalizeSettings(semester string, in SettingsInput) (Settings, error) {
	if err := ValidateSemester(semester); err != nil {
		return Settings{}, err
	}
	if in.ExpectedRevision == nil {
		return Settings{}, &ValidationError{
			Code:    "INVALID_ARGUMENT",
			Message: "expected_revision 必填（首次保存填 0）",
		}
	}
	if *in.ExpectedRevision < 0 {
		return Settings{}, invalid("expected_revision 不能为负数")
	}

	count := DefaultPeriodCount
	if in.PeriodCount != nil {
		count = *in.PeriodCount
	}
	if count != DefaultPeriodCount {
		return Settings{}, &ValidationError{
			Code:    "INVALID_ARGUMENT",
			Message: "河大 v1 固定 13 节，period_count 必须为 13",
			Details: map[string]any{"period_count": count},
		}
	}
	if len(in.Periods) != count {
		return Settings{}, &ValidationError{
			Code:    "INVALID_ARGUMENT",
			Message: "periods 必须包含全部 13 节",
			Details: map[string]any{"got": len(in.Periods), "want": count},
		}
	}

	periods := make([]Period, 0, count)
	seen := map[int]bool{}
	for _, p := range in.Periods {
		if p.Index < 1 || p.Index > count {
			return Settings{}, invalid("节次编号必须在 1–%d 之间", count)
		}
		if seen[p.Index] {
			return Settings{}, invalid("节次编号重复：%d", p.Index)
		}
		seen[p.Index] = true

		normalized, err := normalizePeriod(p)
		if err != nil {
			return Settings{}, err
		}
		periods = append(periods, normalized)
	}
	for i := 1; i <= count; i++ {
		if !seen[i] {
			return Settings{}, invalid("缺少节次：%d", i)
		}
	}
	sortPeriods(periods)
	if err := checkPeriodOrder(periods); err != nil {
		return Settings{}, err
	}

	if in.TotalWeeks != nil && (*in.TotalWeeks < 1 || *in.TotalWeeks > 60) {
		return Settings{}, invalid("total_weeks 必须为 1–60 或 null")
	}

	anchor, err := normalizeAnchor(in.WeekAnchor)
	if err != nil {
		return Settings{}, err
	}

	var label *string
	if in.SemesterLabel != nil {
		trimmed := strings.TrimSpace(*in.SemesterLabel)
		if len([]rune(trimmed)) > 128 {
			return Settings{}, invalid("semester_label 长度不能超过 128")
		}
		if trimmed != "" {
			label = &trimmed
		}
	}

	return Settings{
		SchemaVersion: 1,
		Semester:      semester,
		TimeZone:      TimeZone,
		SemesterLabel: label,
		WeekAnchor:    anchor,
		WeekStartDay:  1,
		TotalWeeks:    in.TotalWeeks,
		PeriodCount:   count,
		Periods:       periods,
	}, nil
}

func normalizePeriod(p Period) (Period, error) {
	hasStart := p.StartTime != nil && strings.TrimSpace(*p.StartTime) != ""
	hasEnd := p.EndTime != nil && strings.TrimSpace(*p.EndTime) != ""
	switch {
	case !hasStart && !hasEnd:
		if p.Configured {
			return Period{}, invalid("第 %d 节标记为已配置但没有时刻", p.Index)
		}
		return Period{Index: p.Index, Configured: false}, nil
	case hasStart != hasEnd:
		return Period{}, invalid("第 %d 节的开始与结束时刻必须同时提供或同时为空", p.Index)
	}

	start, err := parseClock(*p.StartTime)
	if err != nil {
		return Period{}, invalid("第 %d 节开始时刻格式无效", p.Index)
	}
	end, err := parseClock(*p.EndTime)
	if err != nil {
		return Period{}, invalid("第 %d 节结束时刻格式无效", p.Index)
	}
	if !start.Before(end) {
		return Period{}, invalid("第 %d 节的开始时刻必须早于结束时刻", p.Index)
	}
	if !p.Configured {
		return Period{}, invalid("第 %d 节提供了时刻但 configured=false", p.Index)
	}
	s, e := start.Format("15:04"), end.Format("15:04")
	return Period{Index: p.Index, StartTime: &s, EndTime: &e, Configured: true}, nil
}

func normalizeAnchor(a WeekAnchor) (WeekAnchor, error) {
	if a.StartDate == nil || strings.TrimSpace(*a.StartDate) == "" {
		if a.Confirmed || (a.Source != "" && a.Source != "UNKNOWN") {
			return WeekAnchor{}, invalid("没有开学日期时不能标记为已确认或指定来源")
		}
		return WeekAnchor{Source: "UNKNOWN"}, nil
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(*a.StartDate))
	if err != nil {
		return WeekAnchor{}, invalid("week_anchor.start_date 必须是 YYYY-MM-DD")
	}
	if date.Weekday() != time.Monday {
		return WeekAnchor{}, invalid("教学周锚点必须是周一")
	}
	if a.Source != "USER" {
		return WeekAnchor{}, invalid("week_anchor.source 只接受 USER")
	}
	if !a.Confirmed {
		return WeekAnchor{}, invalid("已保存的锚点必须 confirmed=true")
	}
	value := date.Format("2006-01-02")
	return WeekAnchor{StartDate: &value, Source: "USER", Confirmed: true}, nil
}

func parseClock(raw string) (time.Time, error) {
	return time.Parse("15:04", strings.TrimSpace(raw))
}

func sortPeriods(periods []Period) {
	for i := 1; i < len(periods); i++ {
		for j := i; j > 0 && periods[j].Index < periods[j-1].Index; j-- {
			periods[j], periods[j-1] = periods[j-1], periods[j]
		}
	}
}

func checkPeriodOrder(periods []Period) error {
	var prev *Period
	for i := range periods {
		p := periods[i]
		if !p.Configured {
			continue
		}
		if prev != nil {
			prevEnd, _ := parseClock(*prev.EndTime)
			start, _ := parseClock(*p.StartTime)
			if start.Before(prevEnd) {
				return invalid("第 %d 节与第 %d 节时间重叠或倒序", prev.Index, p.Index)
			}
		}
		prev = &periods[i]
	}
	return nil
}

// PeriodTimes returns the configured start/end for the given period indexes.
// ok is false when any requested period is unknown or not configured.
func (s Settings) PeriodTimes(indexes []int) (start string, end string, ok bool) {
	byIndex := map[int]Period{}
	for _, p := range s.Periods {
		byIndex[p.Index] = p
	}
	first, last := 0, 0
	for i, idx := range indexes {
		p, found := byIndex[idx]
		if !found || !p.Configured || p.StartTime == nil || p.EndTime == nil {
			return "", "", false
		}
		if i == 0 {
			first = idx
		}
		last = idx
	}
	return *byIndex[first].StartTime, *byIndex[last].EndTime, true
}

// MissingPeriods lists period indexes that are not configured yet.
func (s Settings) MissingPeriods() []int {
	var out []int
	for _, p := range s.Periods {
		if !p.Configured {
			out = append(out, p.Index)
		}
	}
	return out
}
