// Package henuparse is the mobile parsing boundary for the HENU timetable and
// classroom parsers. It exposes gobind-friendly string/bytes → JSON entry
// points and deliberately imports only pure parsing packages; no HTTP,
// database or desktop runtime travels with it. Error messages are fixed safe
// codes, never the underlying Go error text.
package henuparse

import (
	"encoding/json"
	"errors"
	"fmt"

	"kairos/server/internal/domain/classroom"
	"kairos/server/internal/domain/henu"
)

// Error codes surfaced through the AAR boundary. The Java side maps these to
// its own error envelope; nothing else crosses the bridge.
var (
	ErrUnsupportedFileType = errors.New("UNSUPPORTED_FILE_TYPE")
	ErrInvalidEncoding     = errors.New("INVALID_ENCODING")
	ErrLoginPage           = errors.New("LOGIN_PAGE")
	ErrEmptyTimetable      = errors.New("EMPTY_TIMETABLE")
	ErrUnrecognized        = errors.New("UNRECOGNIZED_DOCUMENT")

	ErrClassroomInvalidEncoding = errors.New("INVALID_ENCODING")
	ErrClassroomLoginPage       = errors.New("LOGIN_PAGE")
	ErrClassroomUnrecognized    = errors.New("UNRECOGNIZED_DOCUMENT")
)

// statsDTO mirrors the parser counters.
type statsDTO struct {
	LogicalCourses    int `json:"logical_courses"`
	Sessions          int `json:"sessions"`
	SourceRows        int `json:"source_rows"`
	UnparsedFragments int `json:"unparsed_fragments"`
}

type sessionDTO struct {
	SessionIndex int    `json:"session_index"`
	Weekday      int    `json:"weekday"`
	Sections     []int  `json:"sections"`
	Weeks        []int  `json:"weeks"`
	Location     string `json:"location"`
	Teacher      string `json:"teacher"`
}

type courseDTO struct {
	CourseIndex        int          `json:"course_index"`
	Name               string       `json:"name"`
	UpstreamCourseCode *string      `json:"upstream_course_code"`
	TeachingClassCode  *string      `json:"teaching_class_code"`
	TeachingClassName  string       `json:"teaching_class_name"`
	Credits            *string      `json:"credits"`
	Teachers           []string     `json:"teachers"`
	Sessions           []sessionDTO `json:"sessions"`
}

type issueDTO struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	CourseIndex *int   `json:"course_index"`
	SourceRow   *int   `json:"source_row"`
}

type resultDTO struct {
	SchemaVersion       int         `json:"schema_version"`
	ParserVersion       string      `json:"parser_version"`
	SemesterLabel       string      `json:"semester_label"`
	DeclaredCourseCount int         `json:"declared_course_count"`
	Stats               statsDTO    `json:"stats"`
	Courses             []courseDTO `json:"courses"`
	Issues              []issueDTO  `json:"issues"`
}

func intPtr(v int) *int { return &v }

// Parse decodes and parses one school timetable export (raw GBK/UTF-8 HTML
// bytes, exactly as served by the school) and returns the serialized DTO.
// It never executes HTML and never fetches anything.
func Parse(raw []byte) (string, error) {
	parsed, err := henu.Parse(raw)
	if err != nil {
		switch {
		case errors.Is(err, henu.ErrUnsupportedFileType):
			return "", ErrUnsupportedFileType
		case errors.Is(err, henu.ErrUndecodable):
			return "", ErrInvalidEncoding
		case errors.Is(err, henu.ErrLoginPage):
			return "", ErrLoginPage
		case errors.Is(err, henu.ErrEmptyTimetable):
			return "", ErrEmptyTimetable
		default:
			return "", ErrUnrecognized
		}
	}

	dto := resultDTO{
		SchemaVersion:       1,
		ParserVersion:       parsed.ParserVersion,
		SemesterLabel:       parsed.SourceSemesterLabel,
		DeclaredCourseCount: parsed.DeclaredCourseCount,
		Stats: statsDTO{
			LogicalCourses:    parsed.Stats.LogicalCourses,
			Sessions:          parsed.Stats.Sessions,
			SourceRows:        parsed.Stats.SourceRows,
			UnparsedFragments: parsed.Stats.UnparsedFragments,
		},
		Courses: []courseDTO{},
		Issues:  []issueDTO{},
	}

	for ci, course := range parsed.Courses {
		c := courseDTO{
			CourseIndex:       ci,
			Name:              course.Name,
			TeachingClassName: course.TeachingClassName,
			Teachers:          append([]string{}, course.Teachers...),
			Sessions:          []sessionDTO{},
		}
		if course.UpstreamCourseCode != "" {
			v := course.UpstreamCourseCode
			c.UpstreamCourseCode = &v
		}
		if course.TeachingClassCode != "" {
			v := course.TeachingClassCode
			c.TeachingClassCode = &v
		}
		if course.Credits != nil {
			c.Credits = course.Credits
		}
		if c.Teachers == nil {
			c.Teachers = []string{}
		}
		for si, s := range course.Sessions {
			c.Sessions = append(c.Sessions, sessionDTO{
				SessionIndex: si,
				Weekday:      s.Weekday,
				Sections:     append([]int{}, s.Sections...),
				Weeks:        append([]int{}, s.Weeks...),
				Location:     s.Location,
				Teacher:      s.Teacher,
			})
		}
		dto.Courses = append(dto.Courses, c)
	}

	for _, issue := range parsed.Issues {
		i := issueDTO{
			Code:     issue.Code,
			Severity: string(issue.Severity),
			Message:  issue.Message,
		}
		if issue.HasCourse {
			i.CourseIndex = intPtr(issue.CourseIndex)
		}
		// Parser rows are 1-based; a zero row means the issue is not tied to
		// a source row (defensive — the parser currently always sets one).
		if issue.Row > 0 {
			i.SourceRow = intPtr(issue.Row)
		}
		dto.Issues = append(dto.Issues, i)
	}

	out, err := json.Marshal(dto)
	if err != nil {
		// Marshal of this DTO cannot fail in practice; keep the boundary
		// safe anyway.
		return "", fmt.Errorf("PARSE_ENCODE_FAILED")
	}
	return string(out), nil
}

// ParseClassroomWeek decodes one classroom week HTML response and returns the
// classroom parser DTO. It is pure and does not touch the school network.
func ParseClassroomWeek(raw []byte) (string, error) {
	parsed, err := classroom.ParseWeek(raw)
	if err != nil {
		switch {
		case errors.Is(err, classroom.ErrInvalidEncoding):
			return "", ErrClassroomInvalidEncoding
		case errors.Is(err, classroom.ErrLoginPage):
			return "", ErrClassroomLoginPage
		default:
			return "", ErrClassroomUnrecognized
		}
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return "", fmt.Errorf("PARSE_ENCODE_FAILED")
	}
	return string(out), nil
}
