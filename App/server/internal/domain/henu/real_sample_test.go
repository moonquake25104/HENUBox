package henu

import (
	"os"
	"testing"
)

// TestRealExportSample parses the user's real export when it is present. It
// prints only aggregate counts and issue codes: no course, teacher, room or
// student values are logged, so the output is safe to paste into a report.
func TestRealExportSample(t *testing.T) {
	path := os.Getenv("HENU_SAMPLE_PATH")
	if path == "" {
		t.Skip("Set HENU_SAMPLE_PATH to an explicitly authorized local sample")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("sample not available: %v", err)
	}

	res, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse real sample: %v", err)
	}

	byCode := map[string]int{}
	for _, issue := range res.Issues {
		byCode[string(issue.Severity)+"/"+issue.Code]++
	}

	t.Logf("bytes=%d parser=%s semester_label=%q declared_courses=%d",
		len(raw), res.ParserVersion, res.SourceSemesterLabel, res.DeclaredCourseCount)
	t.Logf("logical_courses=%d sessions=%d source_rows=%d unparsed_fragments=%d",
		res.Stats.LogicalCourses, res.Stats.Sessions, res.Stats.SourceRows, res.Stats.UnparsedFragments)
	t.Logf("issues=%d %v", len(res.Issues), byCode)

	// Structural invariants that do not depend on how many courses the file has.
	if res.Stats.LogicalCourses == 0 || res.Stats.Sessions == 0 {
		t.Fatalf("no courses parsed")
	}
	if res.Stats.UnparsedFragments != 0 {
		t.Fatalf("unparsed fragments: %d", res.Stats.UnparsedFragments)
	}
	for i, c := range res.Courses {
		if len(c.Sessions) == 0 {
			t.Fatalf("course #%d has no sessions", i+1)
		}
		for j, s := range c.Sessions {
			if s.Weekday < 1 || s.Weekday > 7 || len(s.Sections) == 0 || len(s.Weeks) == 0 {
				t.Fatalf("invalid session: course #%d session #%d", i+1, j+1)
			}
		}
	}
}
