package henu

import (
	"reflect"
	"testing"
)

func TestParseScheduleFragment(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		weekday  int
		sections []int
		weeks    []int
		location string
	}{
		{
			name:    "contiguous sections with capacity",
			raw:     "1-18周 三[11-13] 示例楼甲306(120)",
			weekday: 3, sections: []int{11, 12, 13},
			weeks:    []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
			location: "示例楼甲306",
		},
		{
			name:    "even weeks only",
			raw:     "4-14周(双) 四[11-13]",
			weekday: 4, sections: []int{11, 12, 13},
			weeks: []int{4, 6, 8, 10, 12, 14},
		},
		{
			name:    "odd weeks with full-width brackets",
			raw:     "1-9周（单） 二[3-4] 示例楼乙402（120）",
			weekday: 2, sections: []int{3, 4},
			weeks:    []int{1, 3, 5, 7, 9},
			location: "示例楼乙402",
		},
		{
			name:    "comma separated weeks",
			raw:     "1,2,3周 五[3-5] 示例楼丙302(78)",
			weekday: 5, sections: []int{3, 4, 5},
			weeks:    []int{1, 2, 3},
			location: "示例楼丙302",
		},
		{
			name:    "single week prefix",
			raw:     "第9周 六[6-8]",
			weekday: 6, sections: []int{6, 7, 8},
			weeks: []int{9},
		},
		{
			name:    "sunday and no location",
			raw:     "10-12周 日[1-2]",
			weekday: 7, sections: []int{1, 2},
			weeks: []int{10, 11, 12},
		},
		{
			name:    "non contiguous sections kept as parsed",
			raw:     "1-18周 一[1,2,4] 示例楼丁101(60)",
			weekday: 1, sections: []int{1, 2, 4},
			weeks:    []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
			location: "示例楼丁101",
		},
		{
			name:    "thirteenth period",
			raw:     "1-18周 二[12-13] 示例楼戊203(90)",
			weekday: 2, sections: []int{12, 13},
			weeks:    []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
			location: "示例楼戊203",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, issue := ParseScheduleFragment(tc.raw)
			if issue != nil {
				t.Fatalf("unexpected issue: %+v", issue)
			}
			if got.Weekday != tc.weekday {
				t.Errorf("weekday = %d, want %d", got.Weekday, tc.weekday)
			}
			if !reflect.DeepEqual(got.Sections, tc.sections) {
				t.Errorf("sections = %v, want %v", got.Sections, tc.sections)
			}
			if !reflect.DeepEqual(got.Weeks, tc.weeks) {
				t.Errorf("weeks = %v, want %v", got.Weeks, tc.weeks)
			}
			if got.Location != tc.location {
				t.Errorf("location = %q, want %q", got.Location, tc.location)
			}
		})
	}
}

func TestParseScheduleFragmentRejectsUnknown(t *testing.T) {
	cases := []string{
		"一段没有节次的文字",
		"1-18周 三[abc]",
		"1-18周 八[1-2]",
		"1-99周 三[1-2]",
		"〔1-18周〕三 示例楼",
	}
	for _, raw := range cases {
		if _, issue := ParseScheduleFragment(raw); issue == nil {
			t.Errorf("%q: expected an issue, got none", raw)
		}
	}
}

func TestConsecutiveRuns(t *testing.T) {
	got := ConsecutiveRuns([]int{1, 2, 4, 5, 6, 9})
	want := [][]int{{1, 2}, {4, 5, 6}, {9}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitScheduleFragments(t *testing.T) {
	got := SplitScheduleFragments("1-18周 五[6-8] 示例楼A309(78)；11-16周 五[11-13]")
	if len(got) != 2 {
		t.Fatalf("expected 2 fragments, got %d: %v", len(got), got)
	}
}
