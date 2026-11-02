package academic

import (
	"testing"
	"time"

	"kairos/server/internal/domain/henu"
)

func configuredSettings() Settings {
	s := DefaultSettings("2026S2")
	anchor := "2026-08-31"
	label := "2026-2027学年第一学期"
	s.WeekAnchor = WeekAnchor{StartDate: &anchor, Source: "USER", Confirmed: true}
	s.SemesterLabel = &label
	for i := range s.Periods {
		start := clock(8*60 + i*50)
		end := clock(8*60 + i*50 + 45)
		s.Periods[i].StartTime = &start
		s.Periods[i].EndTime = &end
		s.Periods[i].Configured = true
	}
	return s
}

func sampleResult() *henu.Result {
	return &henu.Result{
		Courses: []henu.Course{
			{
				Name:               "合成课程甲",
				UpstreamCourseCode: "DEMO-001",
				Credits:            strPtr("2.5"),
				Teachers:           []string{"示例教师"},
				Sessions: []henu.Session{
					{Weekday: 4, Sections: []int{11, 12, 13}, Weeks: []int{4, 6, 8}, Location: "示例楼101", Teacher: "示例教师"},
				},
			},
		},
		SourceSemesterLabel: "2026-2027学年第一学期",
		Stats:               henu.Stats{LogicalCourses: 1, Sessions: 1, SourceRows: 1},
		ParserVersion:       henu.ParserVersion,
	}
}

func strPtr(s string) *string { return &s }

func TestBuildPreviewReadyWithConfiguredPeriods(t *testing.T) {
	settings := configuredSettings()
	label := "2026-2027学年第一学期"
	settings.SemesterLabel = &label

	p := BuildPreview(sampleResult(), settings, "2026S2", BaseVersions{SettingsRevision: 1, AggregateRevision: 1}, "p-1", time.Now())
	if p.Status != StatusReady || !p.CanCommit {
		t.Fatalf("status = %s can_commit = %v issues = %+v", p.Status, p.CanCommit, p.Issues)
	}
	s := p.Courses[0].Sessions[0]
	if s.StartTime == nil || s.EndTime == nil || s.TimeState != "CONFIGURED" {
		t.Fatalf("session times = %+v", s)
	}
	if len(p.Diff.Added) != 1 || p.Diff.Added[0] != s.PreviewSessionID {
		t.Fatalf("diff = %+v", p.Diff)
	}
	if p.Summary.LogicalCourses != 1 || p.Summary.Sessions != 1 {
		t.Fatalf("summary = %+v", p.Summary)
	}
}

func TestBuildPreviewNeedsConfigurationWithoutTimes(t *testing.T) {
	settings := configuredSettings()
	for i := range settings.Periods {
		settings.Periods[i] = Period{Index: settings.Periods[i].Index}
	}

	p := BuildPreview(sampleResult(), settings, "2026S2", BaseVersions{}, "p-2", time.Now())
	if p.Status != StatusNeedsConfiguration || p.CanCommit {
		t.Fatalf("status = %s can_commit = %v", p.Status, p.CanCommit)
	}
	if p.Courses[0].Sessions[0].StartTime != nil {
		t.Fatal("times must stay null when periods are not configured")
	}
	found := false
	for _, issue := range p.Issues {
		if issue.Code == henu.CodeTimesRequired && issue.Severity == henu.SeverityBlocking {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing TIMES_REQUIRED issue: %+v", p.Issues)
	}
}

func TestBuildPreviewNeedsReviewOnWarnings(t *testing.T) {
	settings := configuredSettings()
	parsed := sampleResult()
	parsed.Issues = []henu.Issue{{
		Code: henu.CodeMissingLocation, Severity: henu.SeverityWarning,
		Message: "该安排没有教室", HasCourse: true, CourseIndex: 0,
	}}

	p := BuildPreview(parsed, settings, "2026S2", BaseVersions{}, "p-3", time.Now())
	if p.Status != StatusReady {
		t.Fatalf("a plain warning must not block READY, got %s: %+v", p.Status, p.Issues)
	}
	if !p.CanCommit {
		t.Fatal("warnings alone must not clear can_commit")
	}
	if p.Courses[0].PreviewCourseID == "" {
		t.Fatal("expected preview course id")
	}
	if p.Issues[0].PreviewCourseID != p.Courses[0].PreviewCourseID {
		t.Fatalf("issue course id = %q", p.Issues[0].PreviewCourseID)
	}
}

func TestBuildPreviewInvalidOnUnknownSections(t *testing.T) {
	settings := configuredSettings()
	label := "2026-2027学年第一学期"
	settings.SemesterLabel = &label
	parsed := sampleResult()
	parsed.Courses[0].Sessions[0].Sections = []int{12, 13, 14}

	p := BuildPreview(parsed, settings, "2026S2", BaseVersions{}, "p-4", time.Now())
	if p.Status != StatusInvalid || p.CanCommit {
		t.Fatalf("status = %s", p.Status)
	}
	found := false
	for _, issue := range p.Issues {
		if issue.Code == "SECTION_EXCEEDS_PERIOD_COUNT" {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %+v", p.Issues)
	}
}

func TestBuildPreviewBlocksSemesterMismatch(t *testing.T) {
	settings := configuredSettings()
	label := "2025-2026学年第二学期"
	settings.SemesterLabel = &label

	p := BuildPreview(sampleResult(), settings, "2026S2", BaseVersions{}, "p-5", time.Now())
	if p.Status != StatusNeedsReview || p.CanCommit {
		t.Fatalf("status = %s can_commit=%v", p.Status, p.CanCommit)
	}
	found := false
	for _, issue := range p.Issues {
		if issue.Code == henu.CodeSemesterMismatch {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %+v", p.Issues)
	}
}

func TestBuildPreviewWarnsWhenSemesterLabelUnknown(t *testing.T) {
	settings := configuredSettings()
	settings.SemesterLabel = nil
	p := BuildPreview(sampleResult(), settings, "2026S2", BaseVersions{}, "p-6", time.Now())
	if p.Status != StatusNeedsReview {
		t.Fatalf("status = %s", p.Status)
	}
	for _, issue := range p.Issues {
		if issue.Code == henu.CodeSemesterUnverified && issue.Severity != henu.SeverityBlocking {
			t.Fatalf("unverified mapping must be blocking: %+v", issue)
		}
	}
}

func TestBuildPreviewRequiresConfirmedWeekAnchor(t *testing.T) {
	settings := configuredSettings()
	settings.WeekAnchor = WeekAnchor{Source: "UNKNOWN"}

	p := BuildPreview(sampleResult(), settings, "2026S2", BaseVersions{}, "p-7", time.Now())
	if p.Status != StatusNeedsConfiguration || p.CanCommit {
		t.Fatalf("status = %s can_commit = %v", p.Status, p.CanCommit)
	}
	found := false
	for _, issue := range p.Issues {
		if issue.Code == henu.CodeWeekAnchorRequired && issue.Severity == henu.SeverityBlocking {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing WEEK_ANCHOR_REQUIRED: %+v", p.Issues)
	}
}

func TestBuildPreviewBlocksWhenSourceLabelMissing(t *testing.T) {
	parsed := sampleResult()
	parsed.SourceSemesterLabel = ""

	p := BuildPreview(parsed, configuredSettings(), "2026S2", BaseVersions{}, "p-8", time.Now())
	if p.Status != StatusNeedsReview {
		t.Fatalf("status = %s", p.Status)
	}
}
