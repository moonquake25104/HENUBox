package academic

import (
	"fmt"
	"time"

	"kairos/server/internal/domain/henu"
)

// SchemaVersion is the preview/API payload version.
const SchemaVersion = 1

// PreviewStatus values, ordered by blocking severity (INVALID is worst).
const (
	StatusInvalid            = "INVALID"
	StatusNeedsReview        = "NEEDS_REVIEW"
	StatusNeedsConfiguration = "NEEDS_CONFIGURATION"
	StatusReady              = "READY"
	StatusCommitted          = "COMMITTED"
)

// BaseVersions are the versions a preview was computed against.
type BaseVersions struct {
	SettingsRevision  int64 `json:"settings_revision"`
	TimetableRevision int64 `json:"timetable_revision"`
	AggregateRevision int64 `json:"aggregate_revision"`
}

// Summary separates logical courses, arrangements and source rows.
type Summary struct {
	LogicalCourses    int `json:"logical_courses"`
	Sessions          int `json:"sessions"`
	SourceRows        int `json:"source_rows"`
	UnparsedFragments int `json:"unparsed_fragments"`
}

// SourceIdentity carries only the upstream keys that the export actually
// provides. Without verified semantics the confidence stays UNVERIFIED.
type SourceIdentity struct {
	UpstreamCourseCode        *string `json:"upstream_course_code"`
	UpstreamTeachingClassCode *string `json:"upstream_teaching_class_code"`
	Confidence                string  `json:"confidence"`
}

// PreviewSession is one arrangement inside a preview. Preview IDs are local to
// the preview and are replaced by academic IDs only after a commit.
type PreviewSession struct {
	PreviewSessionID   string  `json:"preview_session_id"`
	SessionID          *string `json:"session_id"`
	SourceEntryID      *string `json:"source_entry_id"`
	UpstreamSessionKey *string `json:"upstream_session_key"`
	Weekday            int     `json:"weekday"`
	Sections           []int   `json:"sections"`
	Weeks              []int   `json:"weeks"`
	Date               *string `json:"date"`
	Location           *string `json:"location"`
	Teacher            *string `json:"teacher"`
	StartTime          *string `json:"start_time"`
	EndTime            *string `json:"end_time"`
	TimeState          string  `json:"time_state"`
}

// PreviewCourse groups the arrangements of one logical course.
type PreviewCourse struct {
	PreviewCourseID  string           `json:"preview_course_id"`
	AcademicCourseID *string          `json:"academic_course_id"`
	SourceIdentity   SourceIdentity   `json:"source_identity"`
	Name             string           `json:"name"`
	Credits          *string          `json:"credits"`
	Sessions         []PreviewSession `json:"sessions"`
}

// ChangedSession, MissingSession and AmbiguousSession mirror the contract's
// diff element shapes.
type ChangedSession struct {
	PreviewSessionID string   `json:"preview_session_id"`
	SessionID        string   `json:"session_id"`
	ChangedFields    []string `json:"changed_fields"`
}

type MissingSession struct {
	SessionID        string  `json:"session_id"`
	AcademicCourseID string  `json:"academic_course_id"`
	Decision         *string `json:"decision"`
}

type AmbiguousSession struct {
	PreviewSessionID    string   `json:"preview_session_id"`
	CandidateSessionIDs []string `json:"candidate_session_ids"`
}

// Diff compares the preview with the currently committed source timetable.
// Phase A has no committed source data, so every session is "added".
type Diff struct {
	Added     []string           `json:"added"`
	Changed   []ChangedSession   `json:"changed"`
	Unchanged []string           `json:"unchanged"`
	Missing   []MissingSession   `json:"missing"`
	Ambiguous []AmbiguousSession `json:"ambiguous"`
}

// Preview is the full persisted preview payload.
type Preview struct {
	SchemaVersion       int             `json:"schema_version"`
	PreviewID           string          `json:"preview_id"`
	Semester            string          `json:"semester"`
	Source              string          `json:"source"`
	Acquisition         string          `json:"acquisition"`
	SourceSemesterLabel *string         `json:"source_semester_label"`
	ParserVersion       string          `json:"parser_version"`
	Status              string          `json:"status"`
	CreatedAt           string          `json:"created_at"`
	ExpiresAt           string          `json:"expires_at"`
	SourceObtainedAt    *string         `json:"source_obtained_at"`
	BaseVersions        BaseVersions    `json:"base_versions"`
	Summary             Summary         `json:"summary"`
	CanCommit           bool            `json:"can_commit"`
	Courses             []PreviewCourse `json:"courses"`
	Diff                Diff            `json:"diff"`
	Issues              []henu.Issue    `json:"issues"`
	CommitID            *string         `json:"commit_id,omitempty"`
	// Validation is only set by GET: it tells the client whether the preview
	// still matches the current settings/aggregate revision.
	Validation *Validation `json:"validation,omitempty"`
}

// Validation reports freshness without re-parsing or rebasing anything.
type Validation struct {
	State                    string `json:"state"` // FRESH | STALE
	CurrentAggregateRevision int64  `json:"current_aggregate_revision"`
}

var shanghai = time.FixedZone("CST", 8*60*60)

// FormatTime renders an RFC3339 timestamp in the fixed Asia/Shanghai offset.
func FormatTime(t time.Time) string {
	return t.In(shanghai).Format("2006-01-02T15:04:05+08:00")
}

// PreviewMeta carries the acquisition provenance of one preview build: how the
// raw bytes were obtained and when the school actually served them. A file
// upload reports FILE_IMPORT with a null time; a direct fetch reports
// DIRECT_FETCH with the real response time. Rebase must preserve both.
type PreviewMeta struct {
	Acquisition      string
	SourceObtainedAt *time.Time
}

// BuildPreview normalises a parser result against the semester settings.
//
// Status precedence follows the contract order: INVALID, NEEDS_REVIEW,
// NEEDS_CONFIGURATION, READY.
func BuildPreview(parsed *henu.Result, settings Settings, semester string, base BaseVersions, previewID string, now time.Time) *Preview {
	return buildPreview(parsed, settings, semester, base, previewID, now, PreviewMeta{Acquisition: "FILE_IMPORT"})
}

// BuildPreviewWithPlan additionally computes the real diff against the
// committed source state and applies explicit rebase resolutions.
func BuildPreviewWithPlan(parsed *henu.Result, settings Settings, semester string, base BaseVersions, previewID string, now time.Time, state ExistingState, resolutions []Resolution) (*Preview, *Plan, error) {
	return BuildPreviewWithPlanMeta(parsed, settings, semester, base, previewID, now, state, resolutions, PreviewMeta{Acquisition: "FILE_IMPORT"})
}

// BuildPreviewWithPlanMeta is BuildPreviewWithPlan with explicit acquisition
// metadata; both file uploads and direct fetches share this entry point.
func BuildPreviewWithPlanMeta(parsed *henu.Result, settings Settings, semester string, base BaseVersions, previewID string, now time.Time, state ExistingState, resolutions []Resolution, meta PreviewMeta) (*Preview, *Plan, error) {
	p := buildPreview(parsed, settings, semester, base, previewID, now, meta)
	p.Diff = Diff{
		Added: []string{}, Changed: []ChangedSession{}, Unchanged: []string{},
		Missing: []MissingSession{}, Ambiguous: []AmbiguousSession{},
	}
	plan, err := ComputePlan(p, state, resolutions)
	if err != nil {
		return nil, nil, err
	}
	if len(p.Diff.Ambiguous) > 0 {
		p.Issues = append(p.Issues, henu.Issue{
			Code: "IDENTITY_RESOLUTION_REQUIRED", Severity: henu.SeverityBlocking,
			Message: "有安排无法自动匹配，需要选择关联或新建",
		})
	}
	for _, m := range p.Diff.Missing {
		if m.Decision == nil {
			p.Issues = append(p.Issues, henu.Issue{
				Code: "MISSING_SESSION_DECISION", Severity: henu.SeverityBlocking,
				Message: "来源中缺席的安排需要明确保留或退役",
			})
			break
		}
	}
	p.Status = computeStatus(p.Issues)
	p.CanCommit = p.Status == StatusReady
	return p, plan, nil
}

func buildPreview(parsed *henu.Result, settings Settings, semester string, base BaseVersions, previewID string, now time.Time, meta PreviewMeta) *Preview {
	created := now.In(shanghai)
	p := &Preview{
		SchemaVersion: SchemaVersion,
		PreviewID:     previewID,
		Semester:      semester,
		Source:        "henu",
		Acquisition:   meta.Acquisition,
		ParserVersion: parsed.ParserVersion,
		CreatedAt:     FormatTime(created),
		ExpiresAt:     FormatTime(created.Add(24 * time.Hour)),
		BaseVersions:  base,
		Courses:       []PreviewCourse{},
		Issues:        []henu.Issue{},
		Diff:          Diff{Added: []string{}, Changed: []ChangedSession{}, Unchanged: []string{}, Missing: []MissingSession{}, Ambiguous: []AmbiguousSession{}},
	}
	if meta.SourceObtainedAt != nil {
		obtained := FormatTime(*meta.SourceObtainedAt)
		p.SourceObtainedAt = &obtained
	}
	if parsed.SourceSemesterLabel != "" {
		label := parsed.SourceSemesterLabel
		p.SourceSemesterLabel = &label
	}
	p.Summary = Summary{
		LogicalCourses:    parsed.Stats.LogicalCourses,
		Sessions:          parsed.Stats.Sessions,
		SourceRows:        parsed.Stats.SourceRows,
		UnparsedFragments: parsed.Stats.UnparsedFragments,
	}

	issues := make([]henu.Issue, 0, len(parsed.Issues)+8)
	issues = append(issues, parsed.Issues...)

	// Semester mapping is a human decision, never a silent default: a missing
	// source label, an unregistered target label and a mismatch are all
	// BLOCKING and make the preview NEEDS_REVIEW (not INVALID).
	switch {
	case p.SourceSemesterLabel == nil:
		issues = append(issues, henu.Issue{
			Code:     henu.CodeSemesterUnverified,
			Severity: henu.SeverityBlocking,
			Message:  "导入文件没有学期标签，需要显式确认学期映射",
		})
	case settings.SemesterLabel == nil:
		issues = append(issues, henu.Issue{
			Code:     henu.CodeSemesterUnverified,
			Severity: henu.SeverityBlocking,
			Message:  "尚未登记该学期的学校名称，需要显式确认学期映射",
		})
	case *settings.SemesterLabel != *p.SourceSemesterLabel:
		issues = append(issues, henu.Issue{
			Code:     henu.CodeSemesterMismatch,
			Severity: henu.SeverityBlocking,
			Message:  "导入文件的学期与目标学期不一致，需要显式确认映射",
		})
	}

	// A confirmed week anchor is required before any preview can be READY.
	if settings.WeekAnchor.StartDate == nil || !settings.WeekAnchor.Confirmed {
		issues = append(issues, henu.Issue{
			Code:     henu.CodeWeekAnchorRequired,
			Severity: henu.SeverityBlocking,
			Message:  "尚未确认教学周锚点",
		})
	}

	sessionNo := 0
	for ci := range parsed.Courses {
		course := parsed.Courses[ci]
		pc := PreviewCourse{
			PreviewCourseID: fmt.Sprintf("pc-%d", ci+1),
			SourceIdentity:  buildIdentity(course),
			Name:            course.Name,
			Credits:         course.Credits,
			Sessions:        []PreviewSession{},
		}
		for si := range course.Sessions {
			s := course.Sessions[si]
			sessionNo++
			ps := PreviewSession{
				PreviewSessionID: fmt.Sprintf("ps-%d", sessionNo),
				Weekday:          s.Weekday,
				Sections:         append([]int(nil), s.Sections...),
				Weeks:            append([]int(nil), s.Weeks...),
				TimeState:        "TIMES_REQUIRED",
			}
			if s.Location != "" {
				loc := s.Location
				ps.Location = &loc
			}
			if s.Teacher != "" {
				teacher := s.Teacher
				ps.Teacher = &teacher
			}

			if exceeds := exceedsPeriodCount(s.Sections, settings.PeriodCount); len(exceeds) > 0 {
				issues = append(issues, henu.Issue{
					Code:             "SECTION_EXCEEDS_PERIOD_COUNT",
					Severity:         henu.SeverityBlocking,
					Message:          "节次超出该学期配置的节数",
					PreviewCourseID:  pc.PreviewCourseID,
					PreviewSessionID: ps.PreviewSessionID,
				})
			} else if start, end, ok := settings.PeriodTimes(s.Sections); ok {
				ps.StartTime, ps.EndTime, ps.TimeState = &start, &end, "CONFIGURED"
			} else {
				issues = append(issues, henu.Issue{
					Code:             henu.CodeTimesRequired,
					Severity:         henu.SeverityBlocking,
					Message:          "请补充所需节次的上课时刻",
					PreviewCourseID:  pc.PreviewCourseID,
					PreviewSessionID: ps.PreviewSessionID,
				})
			}

			pc.Sessions = append(pc.Sessions, ps)
			p.Diff.Added = append(p.Diff.Added, ps.PreviewSessionID)
		}
		p.Courses = append(p.Courses, pc)
	}

	// Attach preview IDs to parser issues (they carry a source course index).
	for i := range issues {
		if issues[i].PreviewCourseID == "" && issues[i].HasCourse &&
			issues[i].CourseIndex >= 0 && issues[i].CourseIndex < len(p.Courses) {
			issues[i].PreviewCourseID = p.Courses[issues[i].CourseIndex].PreviewCourseID
		}
	}
	p.Issues = issues
	p.Status = computeStatus(issues)
	p.CanCommit = p.Status == StatusReady
	return p
}

func buildIdentity(c henu.Course) SourceIdentity {
	id := SourceIdentity{Confidence: "NONE"}
	if c.UpstreamCourseCode != "" {
		code := c.UpstreamCourseCode
		id.UpstreamCourseCode = &code
		id.Confidence = "UNVERIFIED"
	}
	if c.TeachingClassCode != "" {
		classCode := c.TeachingClassCode
		id.UpstreamTeachingClassCode = &classCode
		if id.Confidence == "NONE" {
			id.Confidence = "UNVERIFIED"
		}
	}
	return id
}

func exceedsPeriodCount(sections []int, periodCount int) []int {
	var out []int
	for _, s := range sections {
		if s > periodCount {
			out = append(out, s)
		}
	}
	return out
}

// computeStatus classifies by *meaning*, not by severity alone:
//
//	INVALID             structural/syntax problems (cannot be interpreted)
//	NEEDS_REVIEW        blocking human decisions (semester mapping, identity)
//	NEEDS_CONFIGURATION blocking configuration (period times, week anchor)
//	READY               no blocking issue; WARNINGs may remain
//
// A plain WARNING (missing room, unusual credits) never blocks by itself: the
// commit step acknowledges warnings explicitly.
func computeStatus(issues []henu.Issue) string {
	invalid := false
	review := false
	config := false
	for _, is := range issues {
		switch is.Code {
		case henu.CodeTimesRequired, henu.CodeWeekAnchorRequired:
			config = true
		case henu.CodeSemesterUnverified, henu.CodeSemesterMismatch,
			"COURSE_IDENTITY_AMBIGUOUS", "IDENTITY_RESOLUTION_REQUIRED",
			"MISSING_SESSION_DECISION":
			review = true
		case henu.CodeMissingLocation, henu.CodeInvalidCredits, henu.CodeDuplicateRow:
			// Advisory only.
		default:
			if is.Severity == henu.SeverityBlocking {
				invalid = true
			}
		}
	}
	switch {
	case invalid:
		return StatusInvalid
	case review:
		return StatusNeedsReview
	case config:
		return StatusNeedsConfiguration
	default:
		return StatusReady
	}
}

// RecomputeStatus refreshes status/can_commit after issues were filtered (for
// example when the user explicitly confirmed a semester mapping).
func RecomputeStatus(p *Preview) {
	p.Status = computeStatus(p.Issues)
	p.CanCommit = p.Status == StatusReady
}
