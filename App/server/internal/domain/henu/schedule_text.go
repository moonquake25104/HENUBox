package henu

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Weekday numbers follow ISO: Monday=1 .. Sunday=7.
const (
	MinWeek     = 1
	MaxWeek     = 60 // input guard; not a claim about any real semester
	MaxPeriodNo = 40 // input guard; the semester setting decides the real count
)

var (
	sectionBracketRe = regexp.MustCompile(`\[\s*([0-9,，、\-–—~～\s]+?)\s*\]`)
	weekdayTailRe    = regexp.MustCompile(`(?:星期|周|礼拜)?\s*([一二三四五六日天1-7])\s*$`)
	capacityRe       = regexp.MustCompile(`\(\s*(\d{1,4})\s*\)\s*$|（\s*(\d{1,4})\s*）\s*$`)
	oddEvenRe        = regexp.MustCompile(`[（(]\s*([单双])\s*[)）]`)
)

var weekdayNames = map[string]int{
	"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "日": 7, "天": 7,
	"1": 1, "2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7,
}

// ScheduleFragment is one arrangement rule parsed from a time/place cell, e.g.
// "1-18周 三[11-13] 教学楼甲306(120)".
type ScheduleFragment struct {
	Weekday  int
	Sections []int
	Weeks    []int
	Location string
	// Raw is kept for diagnostics only and never serialized to an API response.
	Raw string
}

// SplitScheduleFragments splits on the Chinese and ASCII semicolons that the
// export uses between multiple arrangements.
func SplitScheduleFragments(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '；' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = normalizeSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ParseScheduleFragment parses a single arrangement. It returns ok=false with
// an issue when the fragment cannot be understood; callers must treat that as
// blocking rather than dropping the fragment.
func ParseScheduleFragment(raw string) (ScheduleFragment, *Issue) {
	frag := ScheduleFragment{Raw: raw}
	text := normalizeSpace(raw)
	if text == "" {
		return frag, nil
	}

	loc := sectionBracketRe.FindStringSubmatchIndex(text)
	if loc == nil {
		return frag, newIssue(CodeUnrecognizedFragment, SeverityBlocking, "无法识别上课时间片段")
	}
	before := strings.TrimSpace(text[:loc[0]])
	sectionText := text[loc[2]:loc[3]]
	after := strings.TrimSpace(text[loc[1]:])

	// Weekday is the tail of the text before the bracket.
	wd := weekdayTailRe.FindStringSubmatch(before)
	if wd == nil {
		return frag, newIssue(CodeMissingWeekday, SeverityBlocking, "上课时间缺少星期")
	}
	day, ok := weekdayNames[wd[1]]
	if !ok {
		return frag, newIssue(CodeMissingWeekday, SeverityBlocking, "上课时间缺少星期")
	}
	frag.Weekday = day

	weekText := strings.TrimSpace(before[:len(before)-len(wd[0])])
	weeks, issue := parseWeeks(weekText)
	if issue != nil {
		return frag, issue
	}
	frag.Weeks = weeks

	sections, issue := parseSections(sectionText)
	if issue != nil {
		return frag, issue
	}
	frag.Sections = sections

	frag.Location = cleanLocation(after)
	return frag, nil
}

func parseWeeks(raw string) ([]int, *Issue) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, newIssue(CodeMissingWeeks, SeverityBlocking, "上课时间缺少周次")
	}

	oddEven := ""
	if m := oddEvenRe.FindStringSubmatch(text); m != nil {
		oddEven = m[1]
		text = oddEvenRe.ReplaceAllString(text, "")
	}
	text = strings.ReplaceAll(text, "周", "")
	text = strings.ReplaceAll(text, "第", "")
	text = strings.ReplaceAll(text, "单双周", "")
	text = strings.ReplaceAll(text, " ", "")
	if text == "" {
		return nil, newIssue(CodeMissingWeeks, SeverityBlocking, "上课时间缺少周次")
	}

	set := map[int]struct{}{}
	for _, part := range splitList(text) {
		lo, hi, ok := parseRange(part)
		if !ok {
			return nil, newIssue(CodeUnrecognizedFragment, SeverityBlocking, "周次格式无法识别")
		}
		if lo < MinWeek || hi > MaxWeek || lo > hi {
			return nil, newIssue(CodeWeekOutOfRange, SeverityBlocking, "周次超出允许范围")
		}
		for w := lo; w <= hi; w++ {
			if oddEven == "单" && w%2 == 0 {
				continue
			}
			if oddEven == "双" && w%2 == 1 {
				continue
			}
			set[w] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil, newIssue(CodeMissingWeeks, SeverityBlocking, "上课时间缺少周次")
	}
	return sortedKeys(set), nil
}

func parseSections(raw string) ([]int, *Issue) {
	text := strings.ReplaceAll(raw, " ", "")
	if text == "" {
		return nil, newIssue(CodeMissingSections, SeverityBlocking, "上课时间缺少节次")
	}
	set := map[int]struct{}{}
	for _, part := range splitList(text) {
		lo, hi, ok := parseRange(part)
		if !ok {
			return nil, newIssue(CodeUnrecognizedFragment, SeverityBlocking, "节次格式无法识别")
		}
		if lo < 1 || hi > MaxPeriodNo || lo > hi {
			return nil, newIssue(CodeSectionOutOfRange, SeverityBlocking, "节次超出允许范围")
		}
		for s := lo; s <= hi; s++ {
			set[s] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil, newIssue(CodeMissingSections, SeverityBlocking, "上课时间缺少节次")
	}
	return sortedKeys(set), nil
}

func splitList(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case ',', '，', '、':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func parseRange(part string) (int, int, bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return 0, 0, false
	}
	for _, sep := range []string{"-", "–", "—", "~", "～"} {
		if i := strings.Index(part, sep); i > 0 {
			lo, err1 := strconv.Atoi(strings.TrimSpace(part[:i]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(part[i+len(sep):]))
			if err1 != nil || err2 != nil {
				return 0, 0, false
			}
			return lo, hi, true
		}
	}
	n, err := strconv.Atoi(part)
	if err != nil {
		return 0, 0, false
	}
	return n, n, true
}

func cleanLocation(raw string) string {
	text := normalizeSpace(raw)
	text = strings.Trim(text, " ,，;；")
	if m := capacityRe.FindStringIndex(text); m != nil {
		text = strings.TrimSpace(text[:m[0]])
	}
	text = strings.Trim(text, " ,，;；")
	return text
}

func sortedKeys(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// ConsecutiveRuns splits sorted section numbers into maximal consecutive runs.
// [1,2,4] -> [[1,2],[4]]; the importer stores each run as its own arrangement.
func ConsecutiveRuns(sections []int) [][]int {
	if len(sections) == 0 {
		return nil
	}
	sorted := append([]int(nil), sections...)
	sort.Ints(sorted)

	var out [][]int
	run := []int{sorted[0]}
	for _, s := range sorted[1:] {
		if s == run[len(run)-1]+1 {
			run = append(run, s)
			continue
		}
		out = append(out, run)
		run = []int{s}
	}
	out = append(out, run)
	return out
}
