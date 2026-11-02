//go:build ignore

// Generates the synthetic parse-comparison fixtures for the Android
// instrumentation test: input bytes + expected DTO JSON (or ERROR:<code>).
// Synthetic content only — never the user's real export.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"kairos/server/mobile/henuparse"
)

const headers = `<tr>
<th>上课班级代码</th><th>上课班级名称</th><th>课程</th><th>总学时</th><th>学分</th>
<th>修读性质</th><th>任课教师</th><th>选课状态</th><th>是否跨学科</th><th>教材</th>
<th>上课时间地点</th><th>备注</th></tr>`

func sample(charset string, rows ...string) string {
	return `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">
<html><head><meta http-equiv="Content-Type" content="text/html; charset=` + charset + `">
<title></title></head><body>
<div>2026-2027学年第一学期&ensp;课程门数：` + fmt.Sprint(len(rows)) + `&ensp;</div>
<table><thead>` + headers + `</thead><tbody>` + join(rows) + `</tbody></table>
</body></html>`
}

func join(rows []string) string {
	out := ""
	for i, r := range rows {
		if i > 0 {
			out += "\n"
		}
		out += r
	}
	return out
}

func row(code, class, course, credits, teacher, time string) string {
	return `<tr><td>` + code + `</td><td>` + class + `</td><td>` + course +
		`</td><td>54</td><td>` + credits + `</td><td>初修</td><td>` + teacher +
		`</td><td>选中</td><td>否</td><td>否</td><td>` + time + `</td><td></td></tr>`
}

func main() {
	outDir := os.Args[1]
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	cases := []struct {
		name string
		raw  []byte
	}{
		{"01-basic-utf8.html", []byte(sample("utf-8",
			row("DEMO-001", "示例班", "[DEMO-001]合成课程甲", "3.0", "[T-01]示例教师甲", "1-18周 三[11-13] 示例楼甲306(120)"),
			row("DEMO-002", "示例班", "[DEMO-002]合成课程乙", "2.0", "[T-02]示例教师乙", "4-14周(双) 四[11-13]"),
			row("DEMO-003", "示例班", "无代码合成课程丙", "2.5", "[T-03]示例教师丙、[T-04]示例教师丁",
				"1-18周 五[6-8] 示例楼乙309(78)；11-16周 五[11-13]"),
		))},
		{"02-noncontig.html", []byte(sample("utf-8",
			row("DEMO-010", "示例班", "[DEMO-010]分段课", "1.0", "[T-10]教师", "1-18周 一[1,2,4] 示例楼"),
		))},
		{"03-late-periods.html", []byte(sample("utf-8",
			row("DEMO-011", "示例班", "[DEMO-011]晚间课", "1.0", "[T-11]教师", "2-16周(单) 日[12-13] 示例楼丙502"),
			row("DEMO-012", "示例班", "[DEMO-012]混合周课", "1.5", "[T-12]教师", "1,3,5-8周 二[9-11] 示例楼丁201"),
		))},
		{"04-loginpage.html", []byte(`<html><body><div>河南大学统一身份认证</div><p>请输入密码和验证码</p></body></html>`)},
		{"05-emptytable.html", []byte(sample("utf-8"))},
		{"06-badenc.bin", []byte{0xcf, 0xb5, 0xff, 0xfe, 0x80, 0x81}},
		{"07-unscheduled.html", []byte(sample("utf-8",
			row("DEMO-020", "示例班", "[DEMO-020]正常课", "2", "合成教师", "1-18周 一[1-2] 合成楼101"),
			row("DEMO-021", "示例班", "[DEMO-021]未排课", "2", "合成教师", "1-18周"),
		))},
	}

	for _, c := range cases {
		if err := os.WriteFile(filepath.Join(outDir, c.name), c.raw, 0o644); err != nil {
			panic(err)
		}
		json, err := henuparse.Parse(c.raw)
		expected := json
		if err != nil {
			expected = "ERROR:" + err.Error()
		}
		if err := os.WriteFile(filepath.Join(outDir, c.name+".expected"), []byte(expected), 0o644); err != nil {
			panic(err)
		}
	}
	fmt.Println("fixtures written:", len(cases))
}
