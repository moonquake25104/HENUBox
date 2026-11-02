package henuparse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func classroomSampleRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "..", "runtime", "qa", "henu-classroom-research-20261029")
	if _, err := os.Stat(filepath.Join(root, "raw")); err != nil {
		t.Skipf("classroom research evidence unavailable: %v", err)
	}
	return root
}

func classroomSample(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(classroomSampleRoot(t), "raw", name))
	if err != nil {
		t.Fatalf("read classroom sample: %v", err)
	}
	return b
}

func TestParseClassroomWeekRealCompleteMatrix(t *testing.T) {
	out, err := ParseClassroomWeek(classroomSample(t, "week9-retained-occupied.gbk.html"))
	if err != nil {
		t.Fatalf("ParseClassroomWeek: %v", err)
	}
	var dto map[string]any
	if err := json.Unmarshal([]byte(out), &dto); err != nil {
		t.Fatalf("decode DTO: %v\n%s", err, out)
	}
	if dto["schema_version"].(float64) != 1 || dto["parser_version"] != "henu-classroom-week-v1" {
		t.Fatalf("header = %v", dto)
	}
	rooms := dto["rooms"].([]any)
	if len(rooms) != 9 {
		t.Fatalf("rooms = %d", len(rooms))
	}
	if dto["anonymous_tables"].(float64) != 0 {
		t.Fatalf("anonymous tables = %v", dto["anonymous_tables"])
	}
	first := rooms[0].(map[string]any)
	if first["name"] == "" || first["type"] == "" || first["seats"].(float64) <= 0 {
		t.Fatalf("room metadata = %v", first)
	}
	if len(first["periods"].(map[string]any)) != 13 || len(first["extra_labels"].([]any)) != 2 {
		t.Fatalf("room period shape = %v", first)
	}
}

func TestParseClassroomWeekInitialOmittedRoom(t *testing.T) {
	out, err := ParseClassroomWeek(classroomSample(t, "week9-building24.gbk.html"))
	if err != nil {
		t.Fatalf("ParseClassroomWeek: %v", err)
	}
	if !strings.Contains(out, `"rooms":[`) {
		t.Fatalf("rooms slice missing: %s", out)
	}
	var dto map[string]any
	if err := json.Unmarshal([]byte(out), &dto); err != nil {
		t.Fatalf("decode DTO: %v", err)
	}
	if len(dto["rooms"].([]any)) != 8 {
		t.Fatalf("rooms = %d", len(dto["rooms"].([]any)))
	}
}

func TestParseClassroomWeekPlaceholderAndErrors(t *testing.T) {
	out, err := ParseClassroomWeek(classroomSample(t, "room105-placeholder.gbk.html"))
	if err != nil {
		t.Fatalf("ParseClassroomWeek: %v", err)
	}
	var dto map[string]any
	if err := json.Unmarshal([]byte(out), &dto); err != nil {
		t.Fatalf("decode DTO: %v", err)
	}
	if dto["anonymous_tables"].(float64) != 1 || len(dto["rooms"].([]any)) != 0 {
		t.Fatalf("placeholder = %v", dto)
	}

	if _, err := ParseClassroomWeek(classroomSample(t, "without-cookie-expired.utf8.html")); err != ErrClassroomLoginPage {
		t.Fatalf("login error = %v", err)
	}
	if _, err := ParseClassroomWeek([]byte{0xcf, 0xb5, 0xff, 0xfe, 0x80, 0x81}); err != ErrClassroomInvalidEncoding {
		t.Fatalf("encoding error = %v", err)
	}
	if _, err := ParseClassroomWeek([]byte("<html><body>not a classroom matrix</body></html>")); err != ErrClassroomUnrecognized {
		t.Fatalf("unrecognized error = %v", err)
	}
}
