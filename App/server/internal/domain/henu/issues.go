package henu

// Severity mirrors the API contract: only BLOCKING issues stop a commit.
type Severity string

const (
	SeverityWarning  Severity = "WARNING"
	SeverityBlocking Severity = "BLOCKING"
)

// Issue codes used by the parser and the preview builder.
const (
	CodeUnrecognizedFragment = "UNRECOGNIZED_FRAGMENT"
	CodeMissingWeekday       = "MISSING_WEEKDAY"
	CodeMissingWeeks         = "MISSING_WEEKS"
	CodeMissingSections      = "MISSING_SECTIONS"
	CodeMissingSchedule      = "MISSING_SCHEDULE"
	CodeUnscheduledCourse    = "UNSCHEDULED_COURSE"
	CodeWeekOutOfRange       = "WEEK_OUT_OF_RANGE"
	CodeSectionOutOfRange    = "SECTION_OUT_OF_RANGE"
	CodeMissingLocation      = "MISSING_LOCATION"
	CodeMissingCourseName    = "MISSING_COURSE_NAME"
	CodeInvalidCredits       = "INVALID_CREDITS"
	CodeDuplicateRow         = "DUPLICATE_SOURCE_ROW"
	CodeTimesRequired        = "TIMES_REQUIRED"
	CodeWeekAnchorRequired   = "WEEK_ANCHOR_REQUIRED"
	CodeSemesterMismatch     = "SEMESTER_MAPPING_REQUIRED"
	CodeSemesterUnverified   = "SEMESTER_MAPPING_UNVERIFIED"
	CodeUnsupportedDated     = "UNSUPPORTED_DATED_SESSION"
)

// Issue is one parsing or preview problem. Messages never contain raw HTML,
// student identity fields or full source rows.
type Issue struct {
	Code             string   `json:"code"`
	Severity         Severity `json:"severity"`
	Message          string   `json:"message"`
	Row              int      `json:"-"`
	CourseIndex      int      `json:"-"`
	HasCourse        bool     `json:"-"`
	SessionIndex     int      `json:"-"`
	PreviewCourseID  string   `json:"preview_course_id,omitempty"`
	PreviewSessionID string   `json:"preview_session_id,omitempty"`
}

func newIssue(code string, severity Severity, message string) *Issue {
	return &Issue{Code: code, Severity: severity, Message: message}
}

// HasBlocking reports whether any issue in the slice is blocking.
func HasBlocking(issues []Issue) bool {
	for _, is := range issues {
		if is.Severity == SeverityBlocking {
			return true
		}
	}
	return false
}
