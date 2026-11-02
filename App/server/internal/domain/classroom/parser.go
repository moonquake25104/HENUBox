// Package classroom parses the KINGOSOFT public-query classroom week matrix.
// It is intentionally pure: raw bytes in, DTO out; no HTTP, cookies, database
// or UI state crosses this boundary.
package classroom

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"kairos/server/internal/domain/henu"
)

// Status is the only production classification. The upstream page cannot
// reliably distinguish course/conflict/temporary causes, so every non-free
// value is OCCUPIED and every untrusted cell is UNKNOWN.
type Status string

const (
	StatusFree     Status = "FREE"
	StatusOccupied Status = "OCCUPIED"
	StatusUnknown  Status = "UNKNOWN"
)

// Fixed safe error codes crossing the mobile AAR boundary.
var (
	ErrInvalidEncoding = errors.New("INVALID_ENCODING")
	ErrLoginPage       = errors.New("LOGIN_PAGE")
	ErrUnrecognized    = errors.New("UNRECOGNIZED_DOCUMENT")
)

// Room is one recognized classroom table.
type Room struct {
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	Seats       int                 `json:"seats"`
	Week        int                 `json:"week"`
	Periods     map[string][]Status `json:"periods"`
	ExtraLabels []string            `json:"extra_labels"`
}

// Issue reports a non-fatal structure problem. The parser never turns an
// issue into FREE.
type Issue struct {
	Code    string `json:"code"`
	Room    string `json:"room,omitempty"`
	Period  int    `json:"period,omitempty"`
	Weekday int    `json:"weekday,omitempty"`
}

// WeekMatrix is the parser result for one building/week response.
type WeekMatrix struct {
	SchemaVersion   int     `json:"schema_version"`
	ParserVersion   string  `json:"parser_version"`
	Rooms           []Room  `json:"rooms"`
	AnonymousTables int     `json:"anonymous_tables"`
	Issues          []Issue `json:"issues"`
}

// ParseWeek decodes one GBK/UTF-8 classroom week HTML response.
func ParseWeek(raw []byte) (*WeekMatrix, error) {
	text, err := henu.Decode(raw)
	if err != nil {
		if errors.Is(err, henu.ErrUndecodable) {
			return nil, ErrInvalidEncoding
		}
		return nil, ErrUnrecognized
	}
	if isExpiredScript(text) {
		return nil, ErrLoginPage
	}

	root, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return nil, ErrUnrecognized
	}

	result := &WeekMatrix{
		SchemaVersion: 1,
		ParserVersion: "henu-classroom-week-v1",
		Rooms:         []Room{},
		Issues:        []Issue{},
	}
	names := map[string]int{}
	for _, table := range collectTables(root) {
		if !isClassroomTable(table) {
			continue
		}
		if !validWeekdayHeader(table) {
			return nil, ErrUnrecognized
		}
		room, issues, anonymous := parseClassroomTable(table)
		result.Issues = append(result.Issues, issues...)
		if anonymous {
			result.AnonymousTables++
			continue
		}
		if room == nil {
			continue
		}
		names[room.Name]++
		if names[room.Name] > 1 {
			result.Issues = append(result.Issues, Issue{Code: "DUPLICATE_ROOM", Room: room.Name})
		}
		result.Rooms = append(result.Rooms, *room)
	}
	if len(result.Rooms) == 0 && result.AnonymousTables == 0 {
		return nil, ErrUnrecognized
	}
	return result, nil
}

func isExpiredScript(text string) bool {
	return strings.Contains(text, "凭证已失效，请重新登录") ||
		strings.Contains(text, "凭证已失效")
}

// cell is a table cell after colspan/rowspan expansion bookkeeping.
type cell struct {
	Text    string
	Style   string
	Colspan int
	Rowspan int
}

type table struct {
	Rows [][]cell
}

func collectTables(root *html.Node) []*table {
	var out []*table
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			out = append(out, buildTable(n))
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

func buildTable(node *html.Node) *table {
	t := &table{}
	pending := map[int]*pendingCell{}
	var walkRows func(*html.Node)
	walkRows = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "tr" {
				t.Rows = append(t.Rows, buildRow(c, pending))
				continue
			}
			walkRows(c)
		}
	}
	walkRows(node)
	return t
}

type pendingCell struct {
	remaining int
	cell      cell
}

func buildRow(tr *html.Node, pending map[int]*pendingCell) []cell {
	var row []cell
	col := 0

	fillPending := func() {
		for {
			p, ok := pending[col]
			if !ok {
				return
			}
			row = append(row, p.cell)
			p.remaining--
			if p.remaining <= 0 {
				delete(pending, col)
			}
			col++
		}
	}

	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || (c.Data != "td" && c.Data != "th") {
			continue
		}
		fillPending()
		current := cell{
			Text:    cellText(c),
			Style:   cellStyle(c),
			Colspan: intAttr(c, "colspan", 1),
			Rowspan: intAttr(c, "rowspan", 1),
		}
		for i := 0; i < current.Colspan; i++ {
			row = append(row, current)
			if current.Rowspan > 1 {
				pending[col+i] = &pendingCell{remaining: current.Rowspan - 1, cell: current}
			}
		}
		col += current.Colspan
		fillPending()
	}
	return row
}

func cellText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
			case html.ElementNode:
				switch c.Data {
				case "br":
					b.WriteString(" ")
				case "script", "style":
					continue
				default:
					walk(c)
				}
			}
		}
	}
	walk(n)
	return normalizeSpace(b.String())
}

func cellStyle(n *html.Node) string {
	style, background := "", ""
	for _, a := range n.Attr {
		if a.Key == "style" {
			style = strings.ToLower(strings.Join(strings.Fields(a.Val), ""))
		}
		if a.Key == "bgcolor" {
			background = strings.ToLower(strings.TrimSpace(a.Val))
		}
	}
	if background != "" && !strings.Contains(style, "background") {
		style += ";background-color:" + background
	}
	return style
}

func intAttr(n *html.Node, name string, fallback int) int {
	for _, a := range n.Attr {
		if a.Key == name {
			if v, err := strconv.Atoi(strings.TrimSpace(a.Val)); err == nil && v > 0 && v <= 64 {
				return v
			}
		}
	}
	return fallback
}

func normalizeSpace(s string) string {
	replacer := strings.NewReplacer(
		"\u00a0", " ", "\u2002", " ", "\u2003", " ", "\u2005", " ",
		"\u2007", " ", "\u2009", " ", "\u200a", " ", "\u205f", " ", "\u3000", " ",
		"\t", " ", "\n", " ", "\r", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(s)), " ")
}

func isClassroomTable(t *table) bool {
	if len(t.Rows) < 3 {
		return false
	}
	first := t.Rows[0]
	if len(first) < 5 {
		return false
	}
	return first[0].Text == "教室名称" && first[1].Text == "教室类型" &&
		first[2].Text == "教室容量" && first[3].Text == "周次" &&
		first[4].Text == "空闲节次"
}

func validWeekdayHeader(t *table) bool {
	row := t.Rows[1]
	if len(row) != 8 && len(row) != 12 {
		return false
	}
	row = row[len(row)-8:]
	want := []string{"节次", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日"}
	for i, label := range want {
		if row[i].Text != label || row[i].Colspan != 1 {
			return false
		}
	}
	return true
}

func parseClassroomTable(t *table) (*Room, []Issue, bool) {
	issues := []Issue{}
	periods := map[string][]Status{}
	extraLabels := []string{}
	var firstDataRow []cell

	for _, row := range t.Rows[2:] {
		if len(row) < 5 {
			continue
		}
		label := strings.TrimSpace(row[4].Text)
		if label == "" {
			continue
		}
		if _, ok := formalPeriod(label); ok || isExtraLabel(label) {
			if firstDataRow == nil {
				firstDataRow = row
			}
			if isExtraLabel(label) {
				extraLabels = append(extraLabels, label)
				continue
			}
			period, _ := formalPeriod(label)
			states := make([]Status, 7)
			// A data row is exactly the period label plus seven day cells once
			// the metadata fill is applied. Any other width means the column
			// mapping cannot be trusted, so the whole row stays UNKNOWN.
			if len(row) != 12 {
				issues = append(issues, Issue{Code: "UNEXPECTED_ROW_WIDTH", Period: period})
				for day := range states {
					states[day] = StatusUnknown
				}
			} else {
				for day := 0; day < 7; day++ {
					cell := row[5+day]
					// A merged cell shifts every later column in the row; none
					// of the seven values can be attributed safely.
					if cell.Colspan > 1 || cell.Rowspan > 1 {
						issues = append(issues, Issue{Code: "UNEXPECTED_MERGE", Period: period, Weekday: day + 1})
						for d := range states {
							states[d] = StatusUnknown
						}
						break
					}
					states[day] = status(cell)
					if states[day] == StatusUnknown {
						issues = append(issues, Issue{Code: "UNKNOWN_CELL", Period: period, Weekday: day + 1})
					}
				}
			}
			key := strconv.Itoa(period)
			if _, dup := periods[key]; dup {
				// Neither conflicting observation has precedence. In particular,
				// an initially blank row cannot conceal a later occupied row.
				issues = append(issues, Issue{Code: "DUPLICATE_PERIOD_ROW", Period: period})
				for day := range states {
					states[day] = StatusUnknown
				}
				periods[key] = states
				continue
			}
			periods[key] = states
		}
	}

	if firstDataRow == nil {
		return nil, append(issues, Issue{Code: "NO_PERIOD_ROWS"}), false
	}
	name := strings.TrimSpace(firstDataRow[0].Text)
	kind := strings.TrimSpace(firstDataRow[1].Text)
	seatsText := strings.TrimSpace(firstDataRow[2].Text)
	weekText := strings.TrimSpace(firstDataRow[3].Text)

	if name == "" && kind == "" && seatsText == "" && weekText == "" {
		return nil, append(issues, Issue{Code: "ANONYMOUS_TABLE"}), true
	}
	if name == "" {
		return nil, append(issues, Issue{Code: "MISSING_ROOM_NAME"}), true
	}

	seats, seatErr := strconv.Atoi(seatsText)
	if seatErr != nil {
		issues = append(issues, Issue{Code: "INVALID_SEATS", Room: name})
		seats = 0
	}
	week, weekErr := parseWeekText(weekText)
	if weekErr != nil {
		issues = append(issues, Issue{Code: "INVALID_WEEK", Room: name})
		week = 0
	}

	for period := 1; period <= 13; period++ {
		key := strconv.Itoa(period)
		if _, ok := periods[key]; !ok {
			issues = append(issues, Issue{Code: "MISSING_PERIOD", Room: name, Period: period})
		}
	}

	room := &Room{
		Name:        name,
		Type:        kind,
		Seats:       seats,
		Week:        week,
		Periods:     periods,
		ExtraLabels: extraLabels,
	}
	if room.ExtraLabels == nil {
		room.ExtraLabels = []string{}
	}
	return room, issues, false
}

func formalPeriod(label string) (int, bool) {
	switch label {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13":
		v, err := strconv.Atoi(label)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func isExtraLabel(label string) bool {
	return label == "中午1" || label == "中午2"
}

func parseWeekText(text string) (int, error) {
	if !strings.HasPrefix(text, "第") || !strings.HasSuffix(text, "周") {
		return 0, fmt.Errorf("bad week")
	}
	value := strings.TrimSuffix(strings.TrimPrefix(text, "第"), "周")
	v, err := strconv.Atoi(value)
	if err != nil || v <= 0 || v > 60 {
		return 0, fmt.Errorf("bad week")
	}
	return v, nil
}

func status(c cell) Status {
	if c.Text != "" || c.Colspan > 1 || c.Rowspan > 1 {
		return StatusUnknown
	}
	for _, item := range strings.Split(c.Style, ";") {
		property := strings.SplitN(item, ":", 2)
		if len(property) != 2 || (property[0] != "background-color" && property[0] != "background") {
			continue
		}
		switch property[1] {
		case "red", "#f00", "#ff0000", "rgb(255,0,0)":
			return StatusOccupied
		case "white", "#fff", "#ffffff", "rgb(255,255,255)":
			return StatusFree
		default:
			return StatusUnknown
		}
	}
	return StatusFree
}
