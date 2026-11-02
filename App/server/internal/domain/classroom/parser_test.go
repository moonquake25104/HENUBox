package classroom

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"kairos/server/internal/domain/henu"
)

func sampleRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "..", "..",
		"runtime", "qa", "henu-classroom-research-20261029")
	if _, err := os.Stat(filepath.Join(root, "raw")); err != nil {
		t.Skipf("classroom research evidence unavailable: %v", err)
	}
	return root
}

func sample(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(sampleRoot(t), "raw", name))
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	return b
}

func mustParseWeek(t *testing.T, name string) *WeekMatrix {
	t.Helper()
	parsed, err := ParseWeek(sample(t, name))
	if err != nil {
		t.Fatalf("ParseWeek(%s): %v", name, err)
	}
	return parsed
}

func TestParseRealRetainedOccupiedWeek(t *testing.T) {
	parsed := mustParseWeek(t, "week9-retained-occupied.gbk.html")
	if len(parsed.Rooms) != 9 {
		t.Fatalf("rooms = %d, want 9", len(parsed.Rooms))
	}
	if parsed.AnonymousTables != 0 {
		t.Fatalf("anonymous tables = %d", parsed.AnonymousTables)
	}
	for _, room := range parsed.Rooms {
		if room.Week != 9 {
			t.Fatalf("%s week = %d", room.Name, room.Week)
		}
		if len(room.ExtraLabels) != 2 {
			t.Fatalf("%s extra labels = %v", room.Name, room.ExtraLabels)
		}
		if len(room.Periods) != 13 {
			t.Fatalf("%s periods = %d", room.Name, len(room.Periods))
		}
		for period := 1; period <= 13; period++ {
			states := room.Periods[strconv.Itoa(period)]
			if len(states) != 7 {
				t.Fatalf("%s period %d days = %d", room.Name, period, len(states))
			}
			for _, status := range states {
				if status != StatusFree && status != StatusOccupied {
					t.Fatalf("%s period %d unexpected status %v", room.Name, period, status)
				}
			}
		}
	}
	// The protocol research established exactly 819 formal cells.
	total := 0
	for _, room := range parsed.Rooms {
		total += len(room.Periods) * 7
	}
	if total != 819 {
		t.Fatalf("formal cells = %d, want 819", total)
	}
}

func TestParseRealInitialOmittedRoom(t *testing.T) {
	parsed := mustParseWeek(t, "week9-building24.gbk.html")
	if len(parsed.Rooms) != 8 {
		t.Fatalf("rooms = %d, want 8", len(parsed.Rooms))
	}
}

func TestParseRealSecondWeek(t *testing.T) {
	parsed := mustParseWeek(t, "week10-building24.gbk.html")
	if len(parsed.Rooms) != 9 {
		t.Fatalf("rooms = %d", len(parsed.Rooms))
	}
	for _, room := range parsed.Rooms {
		if room.Week != 10 {
			t.Fatalf("%s week = %d", room.Name, room.Week)
		}
	}
}

func TestParseAnonymousPlaceholder(t *testing.T) {
	parsed := mustParseWeek(t, "room105-placeholder.gbk.html")
	if parsed.AnonymousTables != 1 || len(parsed.Rooms) != 0 {
		t.Fatalf("anonymous=%d rooms=%d", parsed.AnonymousTables, len(parsed.Rooms))
	}
}

func TestParseExpiredScript(t *testing.T) {
	_, err := ParseWeek(sample(t, "without-cookie-expired.utf8.html"))
	if err != ErrLoginPage {
		t.Fatalf("err = %v, want login page", err)
	}
}

func TestParseUnknownStatusNeverFree(t *testing.T) {
	if status(cell{Text: "", Style: "background-color:blue"}) != StatusUnknown {
		t.Fatal("unknown color must not become free")
	}
	if status(cell{Text: "", Colspan: 2}) != StatusUnknown {
		t.Fatal("merged cell must not become free")
	}
	if status(cell{Text: "text"}) != StatusUnknown {
		t.Fatal("non-empty cell must not become free")
	}
	if status(cell{Text: "", Style: "background-color:red"}) != StatusOccupied {
		t.Fatal("red must be occupied")
	}
	if status(cell{Text: ""}) != StatusFree {
		t.Fatal("empty unstyled cell must be free")
	}
}

func TestParseJsonShape(t *testing.T) {
	parsed := mustParseWeek(t, "week9-retained-occupied.gbk.html")
	out, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dto map[string]any
	if err := json.Unmarshal(out, &dto); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dto["schema_version"].(float64) != 1 || dto["parser_version"] != "henu-classroom-week-v1" {
		t.Fatalf("dto header = %v", dto)
	}
	rooms := dto["rooms"].([]any)
	if len(rooms) != 9 {
		t.Fatalf("rooms = %d", len(rooms))
	}
	first := rooms[0].(map[string]any)
	periods := first["periods"].(map[string]any)
	if len(periods) != 13 {
		t.Fatalf("periods = %d", len(periods))
	}
	if first["extra_labels"].([]any) == nil {
		t.Fatal("extra labels must not be null")
	}
}

// The normalized JSON files were produced independently of this parser by the
// protocol research tooling; every cell must agree.
type normalizedRoom struct {
	Name    string              `json:"name"`
	Type    string              `json:"type"`
	Seats   int                 `json:"seats"`
	Week    int                 `json:"week"`
	Periods map[string][]string `json:"periods"`
}

type normalizedMatrix struct {
	Rooms []normalizedRoom `json:"rooms"`
}

func TestParseMatchesNormalizedEvidence(t *testing.T) {
	for _, tc := range []struct{ raw, normalized string }{
		{"week9-retained-occupied.gbk.html", "week9-retained-occupied.json"},
		{"week10-building24.gbk.html", "week10-building24.json"},
	} {
		parsed := mustParseWeek(t, tc.raw)
		data, err := os.ReadFile(filepath.Join(sampleRoot(t), "normalized", tc.normalized))
		if err != nil {
			t.Fatalf("read normalized: %v", err)
		}
		var want normalizedMatrix
		if err := json.Unmarshal(data, &want); err != nil {
			t.Fatalf("decode normalized: %v", err)
		}
		if len(parsed.Issues) != 0 {
			t.Fatalf("%s: unexpected issues %v", tc.raw, parsed.Issues)
		}
		if len(parsed.Rooms) != len(want.Rooms) {
			t.Fatalf("%s: rooms %d want %d", tc.raw, len(parsed.Rooms), len(want.Rooms))
		}
		byName := map[string]Room{}
		for _, room := range parsed.Rooms {
			byName[room.Name] = room
		}
		for _, wantRoom := range want.Rooms {
			got, ok := byName[wantRoom.Name]
			if !ok {
				t.Fatalf("%s: room %s missing", tc.raw, wantRoom.Name)
			}
			if got.Type != wantRoom.Type || got.Seats != wantRoom.Seats || got.Week != wantRoom.Week {
				t.Fatalf("%s: room %s metadata = %+v want %+v", tc.raw, wantRoom.Name, got, wantRoom)
			}
			for period := 1; period <= 13; period++ {
				wantStates, ok := wantRoom.Periods[strconv.Itoa(period)]
				if !ok || len(wantStates) != 7 {
					t.Fatalf("%s: room %s period %d normalized shape", tc.raw, wantRoom.Name, period)
				}
				gotStates := got.Periods[strconv.Itoa(period)]
				if gotStates == nil || len(gotStates) != 7 {
					t.Fatalf("%s: room %s period %d parsed shape", tc.raw, wantRoom.Name, period)
				}
				for day := 0; day < 7; day++ {
					if string(gotStates[day]) != wantStates[day] {
						t.Fatalf("%s: room %s p%d d%d = %s want %s",
							tc.raw, wantRoom.Name, period, day+1, gotStates[day], wantStates[day])
					}
				}
			}
		}
	}
}

// helperFreeRooms extracts the free-room names and the declared count from one
// raw independent free-list response. It is test-only: production never calls
// the helper endpoint to build results.
func helperFreeRooms(t *testing.T, name string) (rooms []string, declared int) {
	t.Helper()
	text, err := henu.Decode(sample(t, name))
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cells = append(cells, cellText(c))
				}
			}
			for i := 0; i+1 < len(cells); i += 2 {
				if cells[i] == "教室" && cells[i+1] == "容量" {
					continue
				}
				fields := strings.Fields(cells[i])
				if len(fields) == 0 || strings.TrimSpace(cells[i+1]) == "" {
					continue
				}
				rooms = append(rooms, fields[len(fields)-1])
			}
			return
		}
		if n.Type == html.TextNode {
			s := strings.TrimSpace(n.Data)
			if strings.HasPrefix(s, "空闲教室数：") {
				if v, err := strconv.Atoi(strings.TrimPrefix(s, "空闲教室数：")); err == nil {
					declared = v
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	return rooms, declared
}

// Every independent free-list condition must agree with the states derived
// from the complete week matrix, exactly as verified during protocol research.
func TestHelperListsCrossCheckMatrix(t *testing.T) {
	parsed := mustParseWeek(t, "week9-retained-occupied.gbk.html")
	freeAt := func(period, weekday int) map[string]bool {
		out := map[string]bool{}
		for _, room := range parsed.Rooms {
			if room.Periods[strconv.Itoa(period)][weekday-1] == StatusFree {
				out[room.Name] = true
			}
		}
		return out
	}
	allFree := func(weekday int) map[string]bool {
		out := map[string]bool{}
		for _, room := range parsed.Rooms {
			all := true
			for period := 1; period <= 13; period++ {
				if room.Periods[strconv.Itoa(period)][weekday-1] != StatusFree {
					all = false
					break
				}
			}
			if all {
				out[room.Name] = true
			}
		}
		return out
	}
	for _, tc := range []struct {
		file       string
		period     int
		weekday    int
		allPeriods bool
		want       []string
	}{
		{"thu-period1.gbk.html", 1, 4, false, []string{"九章南B座105", "九章南B座401", "九章南B座402", "九章南B座403"}},
		{"thu-period3.gbk.html", 3, 4, false, []string{"九章南B座105"}},
		{"fri-period13.gbk.html", 13, 5, false, []string{"九章南B座105", "九章南B座201", "九章南B座302", "九章南B座403"}},
		{"thu-all13.gbk.html", 0, 4, true, []string{"九章南B座105"}},
		{"fri-all13.gbk.html", 0, 5, true, []string{"九章南B座105"}},
	} {
		rooms, declared := helperFreeRooms(t, tc.file)
		got := map[string]bool{}
		for _, name := range rooms {
			got[name] = true
		}
		want := map[string]bool{}
		for _, name := range tc.want {
			want[name] = true
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: helper rooms %v want %v", tc.file, rooms, tc.want)
		}
		var freeSet map[string]bool
		if tc.allPeriods {
			freeSet = allFree(tc.weekday)
		} else {
			freeSet = freeAt(tc.period, tc.weekday)
		}
		if !reflect.DeepEqual(freeSet, want) {
			t.Fatalf("%s: matrix free set %v want %v", tc.file, freeSet, tc.want)
		}
		if declared != len(want) {
			t.Fatalf("%s: declared free count %d want %d", tc.file, declared, len(want))
		}
	}
}

// --- synthetic anomaly fixtures ---

type dayCell struct {
	attrs string
	text  string
}

func matrixTableHTML(dataRows string) string {
	return `<html><head><meta charset="utf-8"><title>t</title></head><body><table>` +
		`<tr><td>教室名称</td><td>教室类型</td><td>教室容量</td><td>周次</td><td>空闲节次</td></tr>` +
		`<tr><td>节次</td><td>星期一</td><td>星期二</td><td>星期三</td><td>星期四</td><td>星期五</td><td>星期六</td><td>星期日</td></tr>` +
		dataRows + `</table></body></html>`
}

// firstRow emits the first data row of a room table: it carries the room
// metadata cells with rowspan=15, exactly like the school response.
func firstRow(label string, days []dayCell) string {
	var b strings.Builder
	b.WriteString(`<tr>`)
	b.WriteString(`<td rowspan="15">测试楼101</td>`)
	b.WriteString(`<td rowspan="15">多媒体教室</td>`)
	b.WriteString(`<td rowspan="15">90</td>`)
	b.WriteString(`<td rowspan="15">第9周</td>`)
	b.WriteString(`<td>` + label + `</td>`)
	for _, d := range days {
		b.WriteString(`<td` + d.attrs + `>` + d.text + `</td>`)
	}
	b.WriteString(`</tr>`)
	return b.String()
}

// contRow emits a later data row: metadata arrives via the rowspan fill, so
// the row contains only the period label and the day cells.
func contRow(label string, days []dayCell) string {
	var b strings.Builder
	b.WriteString(`<tr>`)
	b.WriteString(`<td>` + label + `</td>`)
	for _, d := range days {
		b.WriteString(`<td` + d.attrs + `>` + d.text + `</td>`)
	}
	b.WriteString(`</tr>`)
	return b.String()
}

var (
	emptyCell  = dayCell{}
	redCell    = dayCell{attrs: ` style="background-color:red;"`}
	blueCell   = dayCell{attrs: ` style="background-color:blue;"`}
	mergedCell = dayCell{attrs: ` colspan="2"`}
	spanRow    = dayCell{attrs: ` rowspan="2"`}
)

func findIssue(issues []Issue, code string) *Issue {
	for i := range issues {
		if issues[i].Code == code {
			return &issues[i]
		}
	}
	return nil
}

func TestParseSyntheticMergedDayCell(t *testing.T) {
	// Monday spans periods 1-2 via rowspan; the second row contributes only
	// six cells because Monday is filled by the rowspan. Both rows must end
	// up fully UNKNOWN instead of misattributing shifted columns.
	doc := matrixTableHTML(
		firstRow("1", []dayCell{spanRow, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}) +
			contRow("2", []dayCell{emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}))
	parsed, err := ParseWeek([]byte(doc))
	if err != nil {
		t.Fatalf("ParseWeek: %v", err)
	}
	if len(parsed.Rooms) != 1 {
		t.Fatalf("rooms = %d", len(parsed.Rooms))
	}
	if findIssue(parsed.Issues, "UNEXPECTED_MERGE") == nil {
		t.Fatalf("missing UNEXPECTED_MERGE, issues = %v", parsed.Issues)
	}
	for _, key := range []string{"1", "2"} {
		for day, s := range parsed.Rooms[0].Periods[key] {
			if s != StatusUnknown {
				t.Fatalf("period %s day %d = %s, want UNKNOWN", key, day+1, s)
			}
		}
	}
}

func TestParseSyntheticShortRow(t *testing.T) {
	doc := matrixTableHTML(firstRow("1", []dayCell{
		emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell,
	}))
	parsed, err := ParseWeek([]byte(doc))
	if err != nil {
		t.Fatalf("ParseWeek: %v", err)
	}
	if findIssue(parsed.Issues, "UNEXPECTED_ROW_WIDTH") == nil {
		t.Fatalf("missing UNEXPECTED_ROW_WIDTH, issues = %v", parsed.Issues)
	}
	for day, s := range parsed.Rooms[0].Periods["1"] {
		if s != StatusUnknown {
			t.Fatalf("day %d = %s, want UNKNOWN", day+1, s)
		}
	}
}

func TestParseSyntheticMissingPeriod(t *testing.T) {
	doc := matrixTableHTML(firstRow("1", []dayCell{redCell, redCell, redCell, redCell, redCell, redCell, redCell}) +
		contRow("2", []dayCell{}))
	parsed, err := ParseWeek([]byte(doc))
	if err != nil {
		t.Fatalf("ParseWeek: %v", err)
	}
	missing := 0
	for _, issue := range parsed.Issues {
		if issue.Code == "MISSING_PERIOD" {
			missing++
		}
	}
	if missing != 11 {
		t.Fatalf("MISSING_PERIOD count = %d, want 11", missing)
	}
	for day, s := range parsed.Rooms[0].Periods["1"] {
		if s != StatusOccupied {
			t.Fatalf("day %d = %s, want OCCUPIED", day+1, s)
		}
	}
}

func TestParseSyntheticUnknownColorAndText(t *testing.T) {
	doc := matrixTableHTML(
		firstRow("1", []dayCell{blueCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}) +
			contRow("2", []dayCell{emptyCell, dayCell{text: "班会"}, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}))
	parsed, err := ParseWeek([]byte(doc))
	if err != nil {
		t.Fatalf("ParseWeek: %v", err)
	}
	room := parsed.Rooms[0]
	if room.Periods["1"][0] != StatusUnknown {
		t.Fatalf("blue cell = %s, want UNKNOWN", room.Periods["1"][0])
	}
	if room.Periods["2"][1] != StatusUnknown {
		t.Fatalf("text cell = %s, want UNKNOWN", room.Periods["2"][1])
	}
	if findIssue(parsed.Issues, "UNKNOWN_CELL") == nil {
		t.Fatalf("missing UNKNOWN_CELL, issues = %v", parsed.Issues)
	}
}

func TestParseSyntheticDuplicatePeriodRow(t *testing.T) {
	// Conflicting duplicate rows have no reliable precedence.
	doc := matrixTableHTML(
		firstRow("3", []dayCell{redCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}) +
			contRow("3", []dayCell{emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}))
	parsed, err := ParseWeek([]byte(doc))
	if err != nil {
		t.Fatalf("ParseWeek: %v", err)
	}
	issue := findIssue(parsed.Issues, "DUPLICATE_PERIOD_ROW")
	if issue == nil || issue.Period != 3 {
		t.Fatalf("missing DUPLICATE_PERIOD_ROW, issues = %v", parsed.Issues)
	}
	if parsed.Rooms[0].Periods["3"][0] != StatusUnknown {
		t.Fatalf("duplicate row is trusted: %s", parsed.Rooms[0].Periods["3"][0])
	}
}

func TestCodexDuplicatePeriodFreeFirstNeverWins(t *testing.T) {
	doc := matrixTableHTML(firstRow("3", []dayCell{emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}) +
		contRow("3", []dayCell{redCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}))
	parsed, err := ParseWeek([]byte(doc))
	if err != nil || parsed.Rooms[0].Periods["3"][0] != StatusUnknown {
		t.Fatalf("conflicting duplicate accepted: %v", err)
	}
}

func TestCodexWrongWeekdayHeaderRejected(t *testing.T) {
	doc := matrixTableHTML(firstRow("1", []dayCell{emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}))
	doc = strings.Replace(doc, "星期四", "星期日", 1)
	if _, err := ParseWeek([]byte(doc)); err != ErrUnrecognized {
		t.Fatalf("wrong weekday attribution accepted: %v", err)
	}
}

func TestCodexLegacyBackgroundNeverAssumedFree(t *testing.T) {
	doc := matrixTableHTML(firstRow("1", []dayCell{{attrs: ` bgcolor="blue"`}, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell}))
	parsed, err := ParseWeek([]byte(doc))
	if err != nil || parsed.Rooms[0].Periods["1"][0] != StatusUnknown {
		t.Fatalf("unknown bgcolor accepted: %v", err)
	}
}

func TestParseSyntheticDuplicateRoomTables(t *testing.T) {
	rows := firstRow("1", []dayCell{emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell, emptyCell})
	doc := matrixTableHTML(rows) + matrixTableHTML(rows)
	parsed, err := ParseWeek([]byte(doc))
	if err != nil {
		t.Fatalf("ParseWeek: %v", err)
	}
	if len(parsed.Rooms) != 2 {
		t.Fatalf("rooms = %d, want 2 (identity resolution belongs to the catalog layer)", len(parsed.Rooms))
	}
	if findIssue(parsed.Issues, "DUPLICATE_ROOM") == nil {
		t.Fatalf("missing DUPLICATE_ROOM, issues = %v", parsed.Issues)
	}
}

func TestParseSyntheticMiddayRowsAreNotPeriods(t *testing.T) {
	seven := func(s dayCell) []dayCell {
		return []dayCell{s, s, s, s, s, s, s}
	}
	var b strings.Builder
	for _, label := range []string{"1", "2", "3", "4", "5", "中午1", "中午2", "6", "7", "8", "9", "10", "11", "12", "13"} {
		if label == "1" {
			b.WriteString(firstRow(label, seven(redCell)))
			continue
		}
		if label == "中午1" {
			b.WriteString(contRow(label, seven(redCell)))
			continue
		}
		if label == "6" {
			b.WriteString(contRow(label, seven(emptyCell)))
			continue
		}
		b.WriteString(contRow(label, seven(redCell)))
	}
	parsed, err := ParseWeek([]byte(matrixTableHTML(b.String())))
	if err != nil {
		t.Fatalf("ParseWeek: %v", err)
	}
	room := parsed.Rooms[0]
	if len(room.Periods) != 13 {
		t.Fatalf("periods = %d, want 13", len(room.Periods))
	}
	if !reflect.DeepEqual(room.ExtraLabels, []string{"中午1", "中午2"}) {
		t.Fatalf("extra labels = %v", room.ExtraLabels)
	}
	if room.Periods["6"][0] != StatusFree {
		t.Fatalf("period 6 monday = %s, want FREE from the labeled row", room.Periods["6"][0])
	}
	if len(room.Periods["7"]) != 7 {
		t.Fatal("period 7 must come from its own labeled row")
	}
}
