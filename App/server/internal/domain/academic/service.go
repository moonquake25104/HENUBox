package academic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"kairos/server/internal/domain/calendar"
	"kairos/server/internal/store"
)

// TxBeginner is satisfied by *pgxpool.Pool and *pgx.Conn.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Service is the single write path for academic-related mutations and for the
// legacy course writes that must share the same version rules.
type Service struct {
	Q    *store.Queries
	Pool TxBeginner
	Log  *slog.Logger
}

// TxOptions selects which counters a changed transaction should bump.
type TxOptions struct {
	BumpSettings  bool
	BumpTimetable bool
}

// Tx runs fn inside one transaction that first locks the version rows for all
// given semesters (byte order, so concurrent multi-semester writes cannot
// deadlock). When fn reports a real change, the requested counters are bumped
// once per partition and the transaction commits.
func (s *Service) Tx(ctx context.Context, owner string, semesters []string, opts TxOptions, fn func(q *store.Queries) (bool, error)) error {
	_, err := s.TxWithVersions(ctx, owner, semesters, opts, fn)
	return err
}

// TxWithVersions runs the transaction and returns the post-commit version
// counters of the first (sorted) partition, read inside the same transaction.
func (s *Service) TxWithVersions(ctx context.Context, owner string, semesters []string, opts TxOptions, fn func(q *store.Queries) (bool, error)) (store.AcademicVersions, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return store.AcademicVersions{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := s.Q.WithTx(tx)

	if err := LockPartitions(ctx, qtx, owner, semesters); err != nil {
		return store.AcademicVersions{}, err
	}
	changed, err := fn(qtx)
	if err != nil {
		return store.AcademicVersions{}, err
	}
	sorted := uniqueSorted(semesters)
	var out store.AcademicVersions
	bumped := changed && (opts.BumpSettings || opts.BumpTimetable)
	if bumped {
		for _, semester := range sorted {
			var settingsDelta, timetableDelta int64
			if opts.BumpSettings {
				settingsDelta = 1
			}
			if opts.BumpTimetable {
				timetableDelta = 1
			}
			v, err := qtx.BumpAcademicVersions(ctx, owner, semester, settingsDelta, timetableDelta, 1)
			if err != nil {
				return store.AcademicVersions{}, err
			}
			if semester == sorted[0] {
				out = v
			}
		}
	} else if len(sorted) > 0 {
		out, err = qtx.GetAcademicVersions(ctx, owner, sorted[0])
		if err != nil {
			return store.AcademicVersions{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AcademicVersions{}, err
	}
	return out, nil
}

// LockPartitions ensures and locks every version row in deterministic order.
func LockPartitions(ctx context.Context, q *store.Queries, owner string, semesters []string) error {
	for _, semester := range uniqueSorted(semesters) {
		if _, err := q.LockAcademicVersions(ctx, owner, semester); err != nil {
			return err
		}
	}
	return nil
}

func uniqueSorted(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Values is the normalized shape of both source_values and effective_values.
type Values struct {
	Name      string  `json:"name"`
	Teacher   *string `json:"teacher"`
	Location  *string `json:"location"`
	Color     *string `json:"color"`
	Hidden    bool    `json:"hidden"`
	Weekday   int     `json:"weekday"`
	Sections  []int   `json:"sections"`
	Weeks     []int   `json:"weeks"`
	Date      *string `json:"date"`
	StartTime *string `json:"start_time"`
	EndTime   *string `json:"end_time"`
}

// Overrides is the explicit personal override set for one session.
type Overrides struct {
	SetFields []string
	Name      *string
	Teacher   *string
	Location  *string
	Color     *string
	Hidden    *bool
}

// Effective applies overrides on top of the immutable source values. Fields not
// in SetFields follow the school; an empty string or false is a real override.
func Effective(source Values, ov Overrides) Values {
	out := source
	out.Hidden = false
	for _, f := range ov.SetFields {
		switch f {
		case "name":
			if ov.Name != nil {
				out.Name = *ov.Name
			}
		case "teacher":
			out.Teacher = ov.Teacher
		case "location":
			out.Location = ov.Location
		case "color":
			out.Color = ov.Color
		case "hidden":
			if ov.Hidden != nil {
				out.Hidden = *ov.Hidden
			}
		}
	}
	return out
}

// Projection is what the legacy courses row must contain for one session.
type Projection struct {
	Name              string
	DayOfWeek         int32
	StartTime         string
	EndTime           string
	WeekPattern       string
	SemesterStartDate string
	Location          string
	Teacher           string
	Color             string
	Hidden            bool
}

// ErrUnsupportedDatedSession is returned when a session carries a real date
// that cannot be represented by the legacy week grid.
var ErrUnsupportedDatedSession = errors.New("UNSUPPORTED_DATED_SESSION")

// BuildProjection derives the legacy course projection for one session.
func BuildProjection(source, effective Values, settings Settings) (Projection, error) {
	if source.Date != nil && *source.Date != "" {
		return Projection{}, ErrUnsupportedDatedSession
	}
	if settings.WeekAnchor.StartDate == nil || !settings.WeekAnchor.Confirmed {
		return Projection{}, &ValidationError{Code: "WEEK_ANCHOR_REQUIRED", Message: "尚未确认教学周锚点"}
	}
	start, end, ok := settings.PeriodTimes(source.Sections)
	if !ok {
		return Projection{}, &ValidationError{Code: "TIMES_REQUIRED", Message: "请补充所需节次的上课时刻"}
	}
	pattern, ok := WeekPatternFromWeeks(source.Weeks)
	if !ok {
		return Projection{}, &ValidationError{Code: "INVALID_SCHEDULE", Message: "周次无法投影到旧课程格式"}
	}
	return Projection{
		Name:              effective.Name,
		DayOfWeek:         int32(source.Weekday),
		StartTime:         start,
		EndTime:           end,
		WeekPattern:       pattern,
		SemesterStartDate: *settings.WeekAnchor.StartDate,
		Location:          deref(effective.Location),
		Teacher:           deref(effective.Teacher),
		Color:             colorOr(effective.Color),
		Hidden:            effective.Hidden,
	}, nil
}

// WeekPatternFromWeeks renders explicit weeks ("1周,3周,5周") and verifies the
// result against the existing calendar matcher for weeks 1..60.
func WeekPatternFromWeeks(weeks []int) (string, bool) {
	if len(weeks) == 0 {
		return "", false
	}
	uniq := map[int]struct{}{}
	for _, w := range weeks {
		if w < 1 || w > 60 {
			return "", false
		}
		uniq[w] = struct{}{}
	}
	parts := make([]string, 0, len(uniq))
	for w := 1; w <= 60; w++ {
		if _, ok := uniq[w]; ok {
			parts = append(parts, strconv.Itoa(w)+"周")
		}
	}
	pattern := strings.Join(parts, ",")
	for w := int64(1); w <= 60; w++ {
		_, want := uniq[int(w)]
		if calendar.MatchesWeekPattern(pattern, w) != want {
			return "", false
		}
	}
	return pattern, true
}

// SourceValuesFromJSON decodes a stored source revision.
func SourceValuesFromJSON(raw []byte) (Values, error) {
	var v Values
	if err := json.Unmarshal(raw, &v); err != nil {
		return Values{}, fmt.Errorf("decode source values: %w", err)
	}
	return v, nil
}

// OverridesFromRow converts the store row to the domain shape.
func OverridesFromRow(setFields []string, name, teacher, location, color *string, hidden *bool) Overrides {
	return Overrides{SetFields: setFields, Name: name, Teacher: teacher, Location: location, Color: color, Hidden: hidden}
}

// ProjectSession writes the effective values of one linked session into its
// legacy course row. Missing settings yield TIMES_REQUIRED/WEEK_ANCHOR_REQUIRED.
func ProjectSession(ctx context.Context, q *store.Queries, owner string, row store.SourceSessionRow) (Projection, error) {
	source, err := SourceValuesFromJSON(row.SourceValues)
	if err != nil {
		return Projection{}, err
	}
	settings, err := loadSettingsForProjection(ctx, q, owner, row.Semester)
	if err != nil {
		return Projection{}, err
	}
	effective := Effective(source, OverridesFromRow(row.SetFields, row.Name, row.Teacher, row.Location, row.Color, row.Hidden))
	// A retired arrangement stays hidden even if the user has a personal
	// override; retirement is source state, not a personal preference.
	if !row.Active {
		effective.Hidden = true
	}
	proj, err := BuildProjection(source, effective, settings)
	if err != nil {
		return Projection{}, err
	}
	if row.LegacyCourseID == nil {
		// No legacy row yet: still return the computed projection so the caller
		// can create the course (commit path).
		return proj, nil
	}
	if err := q.TouchCourseProjection(ctx, *row.LegacyCourseID, proj.Name, proj.DayOfWeek,
		proj.StartTime, proj.EndTime, proj.WeekPattern, proj.SemesterStartDate,
		proj.Location, proj.Teacher, proj.Color, row.Semester, proj.Hidden); err != nil {
		return Projection{}, err
	}
	return proj, nil
}

// SettingsFromStoreRow converts the persisted settings row to the domain type.
func SettingsFromStoreRow(row store.AcademicSettingsRow) (Settings, error) {
	var periods []Period
	if err := json.Unmarshal(row.Periods, &periods); err != nil {
		return Settings{}, err
	}
	out := Settings{
		SchemaVersion: 1,
		Semester:      row.Semester,
		Revision:      row.Revision,
		TimeZone:      row.TimeZone,
		SemesterLabel: row.SemesterLabel,
		WeekAnchor: WeekAnchor{
			Source:    row.WeekAnchorSource,
			Confirmed: row.WeekAnchorConfirmed,
		},
		WeekStartDay: int(row.WeekStartDay),
		PeriodCount:  int(row.PeriodCount),
		Periods:      periods,
	}
	if row.WeekAnchorDate != nil {
		v := row.WeekAnchorDate.Format("2006-01-02")
		out.WeekAnchor.StartDate = &v
	}
	if row.TotalWeeks != nil {
		v := int(*row.TotalWeeks)
		out.TotalWeeks = &v
	}
	return out, nil
}

func loadSettingsForProjection(ctx context.Context, q *store.Queries, owner, semester string) (Settings, error) {
	row, err := q.GetAcademicSettings(ctx, owner, semester)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, &ValidationError{Code: "TIMES_REQUIRED", Message: "尚未配置该学期的作息"}
	}
	if err != nil {
		return Settings{}, err
	}
	return SettingsFromStoreRow(row)
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func colorOr(p *string) string {
	if p == nil || *p == "" {
		return "#7C8CC0"
	}
	return *p
}
