// Fingerprints a private HENU sample without printing or writing course rows.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"kairos/server/mobile/henuparse"
	"os"
	"sort"
)

func canonical(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if e.Encode(v) != nil {
		panic("canonical encoding failed")
	}
	return bytes.TrimSpace(b.Bytes())
}

func main() {
	if len(os.Args) != 2 {
		panic("expected a private sample path")
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic("private sample unavailable")
	}
	output, err := henuparse.Parse(raw)
	if err != nil {
		panic("private sample parse failed")
	}
	var parsed struct {
		SemesterLabel string         `json:"semester_label"`
		Stats         map[string]int `json:"stats"`
		Courses       []struct {
			Name      string   `json:"name"`
			Code      *string  `json:"upstream_course_code"`
			ClassCode *string  `json:"teaching_class_code"`
			Credits   *string  `json:"credits"`
			Teachers  []string `json:"teachers"`
			Sessions  []struct {
				Weekday  int    `json:"weekday"`
				Sections []int  `json:"sections"`
				Weeks    []int  `json:"weeks"`
				Location string `json:"location"`
				Teacher  string `json:"teacher"`
			} `json:"sessions"`
		} `json:"courses"`
	}
	if json.Unmarshal([]byte(output), &parsed) != nil {
		panic("parser DTO is invalid")
	}
	rows := []string{}
	for _, c := range parsed.Courses {
		sort.Strings(c.Teachers)
		for _, s := range c.Sessions {
			sort.Ints(s.Sections)
			sort.Ints(s.Weeks)
			rows = append(rows, string(canonical([]any{c.Code, c.ClassCode, c.Credits, c.Name, c.Teachers, s.Weekday, s.Sections, s.Weeks, s.Location, s.Teacher})))
		}
	}
	sort.Strings(rows)
	digest := sha256.Sum256(canonical([]any{parsed.SemesterLabel, rows}))
	result := map[string]any{"schema_version": 1, "fingerprint": hex.EncodeToString(digest[:]), "stats": parsed.Stats, "arrangements": len(rows), "private_rows_saved": false}
	fmt.Println(string(canonical(result)))
}
