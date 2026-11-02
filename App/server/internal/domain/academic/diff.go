package academic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// ExistingCourse / ExistingSession describe the committed source state used to
// compute a real diff. They are loaded from academic_* tables, never parsed
// from the upload.
type ExistingCourse struct {
	ID        string
	Name      string
	Code      *string
	ClassCode *string
	Credits   *string
}

type ExistingSession struct {
	SessionID      string
	CourseID       string
	Active         bool
	Values         Values
	LegacyCourseID *int64
}

type ExistingState struct {
	Courses  []ExistingCourse
	Sessions []ExistingSession
}

// Decision on an arrangement that is present in the committed timetable but
// absent from the uploaded export.
const (
	DecisionRetain = "RETAIN"
	DecisionRetire = "RETIRE"
)

// Resolution kinds mirror the API contract.
const (
	KindLinkCourse    = "LINK_COURSE"
	KindCreateCourse  = "CREATE_COURSE"
	KindLinkSession   = "LINK_SESSION"
	KindCreateSession = "CREATE_SESSION"
	KindRetainSession = "RETAIN_SESSION"
	KindRetireSession = "RETIRE_SESSION"
)

// Resolution is one explicit user decision from a rebase request.
type Resolution struct {
	Kind             string `json:"kind"`
	PreviewCourseID  string `json:"preview_course_id,omitempty"`
	PreviewSessionID string `json:"preview_session_id,omitempty"`
	AcademicCourseID string `json:"academic_course_id,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
}

// Plan is the diff plus the identity decisions that produced it.
type Plan struct {
	CourseMatches    map[string]string // preview_course_id  -> academic_course_id
	SessionMatches   map[string]string // preview_session_id -> session_id
	MissingDecisions map[string]string // session_id -> RETAIN|RETIRE
	Unresolved       bool
}

// ComputePlan fills diff/identity fields on the preview from the committed
// state and any explicit resolutions. It never mutates the source data.
func ComputePlan(p *Preview, state ExistingState, resolutions []Resolution) (*Plan, error) {
	plan := &Plan{
		CourseMatches:    map[string]string{},
		SessionMatches:   map[string]string{},
		MissingDecisions: map[string]string{},
	}

	byCourseKey := map[string][]ExistingCourse{}
	for _, c := range state.Courses {
		byCourseKey[courseMatchKey(c.Name, c.Code, c.ClassCode, c.Credits)] = append(byCourseKey[courseMatchKey(c.Name, c.Code, c.ClassCode, c.Credits)], c)
	}
	byCourseID := map[string]ExistingCourse{}
	for _, c := range state.Courses {
		byCourseID[c.ID] = c
	}

	// Explicit course resolutions first. Every ID must belong to this preview /
	// state, and targets are strictly one-to-one.
	previewCourses := map[string]*PreviewCourse{}
	previewSessions := map[string]*PreviewSession{}
	for i := range p.Courses {
		previewCourses[p.Courses[i].PreviewCourseID] = &p.Courses[i]
		for j := range p.Courses[i].Sessions {
			previewSessions[p.Courses[i].Sessions[j].PreviewSessionID] = &p.Courses[i].Sessions[j]
		}
	}
	linkCourse := map[string]string{}
	createCourse := map[string]bool{}
	linkSession := map[string]string{}
	createSession := map[string]bool{}
	retain := map[string]bool{}
	retire := map[string]bool{}
	usedCourseTargets := map[string]bool{}
	usedSessionTargets := map[string]bool{}

	for _, r := range resolutions {
		switch r.Kind {
		case KindLinkCourse:
			if r.PreviewCourseID == "" || r.AcademicCourseID == "" {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "LINK_COURSE 需要 preview_course_id 与 academic_course_id"}
			}
			if _, ok := previewCourses[r.PreviewCourseID]; !ok {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "LINK_COURSE 的 preview_course_id 不属于该预览"}
			}
			if _, ok := byCourseID[r.AcademicCourseID]; !ok {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "LINK_COURSE 目标不存在于当前学期来源"}
			}
			if usedCourseTargets[r.AcademicCourseID] {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "LINK_COURSE 目标被重复使用"}
			}
			usedCourseTargets[r.AcademicCourseID] = true
			linkCourse[r.PreviewCourseID] = r.AcademicCourseID
		case KindCreateCourse:
			if _, ok := previewCourses[r.PreviewCourseID]; !ok {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "CREATE_COURSE 的 preview_course_id 不属于该预览"}
			}
			createCourse[r.PreviewCourseID] = true
		case KindLinkSession:
			if r.PreviewSessionID == "" || r.SessionID == "" {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "LINK_SESSION 需要 preview_session_id 与 session_id"}
			}
			if _, ok := previewSessions[r.PreviewSessionID]; !ok {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "LINK_SESSION 的 preview_session_id 不属于该预览"}
			}
			if usedSessionTargets[r.SessionID] {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "LINK_SESSION 目标被重复使用"}
			}
			usedSessionTargets[r.SessionID] = true
			linkSession[r.PreviewSessionID] = r.SessionID
		case KindCreateSession:
			if _, ok := previewSessions[r.PreviewSessionID]; !ok {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "CREATE_SESSION 的 preview_session_id 不属于该预览"}
			}
			createSession[r.PreviewSessionID] = true
		case KindRetainSession:
			if r.SessionID == "" {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "RETAIN_SESSION 需要 session_id"}
			}
			retain[r.SessionID] = true
		case KindRetireSession:
			if r.SessionID == "" {
				return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "RETIRE_SESSION 需要 session_id"}
			}
			retire[r.SessionID] = true
		default:
			return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "未知的 resolution 类型"}
		}
	}
	for id := range linkCourse {
		if createCourse[id] {
			return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "同一逻辑课不能同时 LINK 和 CREATE"}
		}
	}
	for id := range linkSession {
		if createSession[id] {
			return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "同一安排不能同时 LINK 和 CREATE"}
		}
	}
	for id := range retain {
		if retire[id] {
			return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "同一安排不能既 RETAIN 又 RETIRE"}
		}
	}

	sessionsByCourse := map[string][]ExistingSession{}
	for _, s := range state.Sessions {
		sessionsByCourse[s.CourseID] = append(sessionsByCourse[s.CourseID], s)
	}

	for ci := range p.Courses {
		pc := &p.Courses[ci]
		var matched *ExistingCourse
		switch {
		case linkCourse[pc.PreviewCourseID] != "":
			c := byCourseID[linkCourse[pc.PreviewCourseID]]
			matched = &c
		case createCourse[pc.PreviewCourseID]:
			matched = nil
		default:
			key := courseMatchKey(pc.Name, pc.SourceIdentity.UpstreamCourseCode, pc.SourceIdentity.UpstreamTeachingClassCode, pc.Credits)
			if candidates := byCourseKey[key]; len(candidates) == 1 {
				c := candidates[0]
				matched = &c
			}
		}
		if matched != nil {
			plan.CourseMatches[pc.PreviewCourseID] = matched.ID
			pc.AcademicCourseID = &matched.ID
		}

		existing := []ExistingSession{}
		if matched != nil {
			existing = sessionsByCourse[matched.ID]
		}
		activeByHash := map[string][]ExistingSession{}
		retiredByHash := map[string][]ExistingSession{}
		for _, s := range existing {
			if s.Active {
				activeByHash[SessionContentHash(s.Values)] = append(activeByHash[SessionContentHash(s.Values)], s)
			} else {
				retiredByHash[SessionContentHash(s.Values)] = append(retiredByHash[SessionContentHash(s.Values)], s)
			}
		}
		matchedExisting := map[string]bool{}

		for si := range pc.Sessions {
			ps := &pc.Sessions[si]
			if id, ok := linkSession[ps.PreviewSessionID]; ok {
				found := false
				for _, s := range existing {
					if s.SessionID == id {
						found = true
						plan.SessionMatches[ps.PreviewSessionID] = id
						matchedExisting[id] = true
						ps.SessionID = &id
						changed := changedFields(s.Values, valuesFromPreview(ps))
						if len(changed) > 0 {
							p.Diff.Changed = append(p.Diff.Changed, ChangedSession{
								PreviewSessionID: ps.PreviewSessionID, SessionID: id, ChangedFields: changed,
							})
						} else {
							p.Diff.Unchanged = append(p.Diff.Unchanged, ps.PreviewSessionID)
						}
					}
				}
				if !found {
					return nil, &ValidationError{Code: "INVALID_ARGUMENT", Message: "LINK_SESSION 目标不属于该逻辑课"}
				}
				continue
			}
			if createSession[ps.PreviewSessionID] {
				p.Diff.Added = append(p.Diff.Added, ps.PreviewSessionID)
				continue
			}
			hash := SessionContentHash(valuesFromPreview(ps))
			candidates := activeByHash[hash]
			available := make([]ExistingSession, 0, len(candidates))
			for _, c := range candidates {
				if !matchedExisting[c.SessionID] {
					available = append(available, c)
				}
			}
			switch len(available) {
			case 1:
				id := available[0].SessionID
				plan.SessionMatches[ps.PreviewSessionID] = id
				matchedExisting[id] = true
				ps.SessionID = &id
				p.Diff.Unchanged = append(p.Diff.Unchanged, ps.PreviewSessionID)
			case 0:
				// A retired arrangement with the same content must be confirmed
				// with an explicit LINK, never revived automatically.
				if retired := retiredByHash[hash]; len(retired) > 0 {
					ids := make([]string, 0, len(retired))
					for _, c := range retired {
						ids = append(ids, c.SessionID)
					}
					sort.Strings(ids)
					p.Diff.Ambiguous = append(p.Diff.Ambiguous, AmbiguousSession{
						PreviewSessionID: ps.PreviewSessionID, CandidateSessionIDs: ids,
					})
				} else {
					p.Diff.Added = append(p.Diff.Added, ps.PreviewSessionID)
				}
			default:
				ids := make([]string, 0, len(available))
				for _, c := range available {
					ids = append(ids, c.SessionID)
				}
				sort.Strings(ids)
				p.Diff.Ambiguous = append(p.Diff.Ambiguous, AmbiguousSession{
					PreviewSessionID: ps.PreviewSessionID, CandidateSessionIDs: ids,
				})
			}
		}

		// Unmatched active sessions of a matched course are missing candidates.
		if matched != nil {
			for _, s := range existing {
				if matchedExisting[s.SessionID] || !s.Active {
					continue
				}
				decision := (*string)(nil)
				if retain[s.SessionID] {
					d := DecisionRetain
					decision = &d
				} else if retire[s.SessionID] {
					d := DecisionRetire
					decision = &d
				}
				if decision != nil {
					plan.MissingDecisions[s.SessionID] = *decision
				}
				p.Diff.Missing = append(p.Diff.Missing, MissingSession{
					SessionID: s.SessionID, AcademicCourseID: matched.ID, Decision: decision,
				})
			}
		}
	}

	// A whole course that disappeared from the export still needs per-session
	// decisions; otherwise its arrangements would silently stay active forever.
	matchedCourses := map[string]bool{}
	for _, id := range plan.CourseMatches {
		matchedCourses[id] = true
	}
	for _, s := range state.Sessions {
		if !s.Active || matchedCourses[s.CourseID] {
			continue
		}
		decision := (*string)(nil)
		if retain[s.SessionID] {
			d := DecisionRetain
			decision = &d
		} else if retire[s.SessionID] {
			d := DecisionRetire
			decision = &d
		}
		if decision != nil {
			plan.MissingDecisions[s.SessionID] = *decision
		}
		p.Diff.Missing = append(p.Diff.Missing, MissingSession{
			SessionID: s.SessionID, AcademicCourseID: s.CourseID, Decision: decision,
		})
	}

	// A course that could not be matched but has several same-name candidates
	// cannot be silently merged into one of them.
	if len(p.Diff.Ambiguous) > 0 {
		plan.Unresolved = true
	}
	for _, m := range p.Diff.Missing {
		if m.Decision == nil {
			plan.Unresolved = true
		}
	}
	return plan, nil
}

// SessionContentHash is a version fingerprint of the school source fields only.
// Derived times and personal overrides are excluded.
func SessionContentHash(v Values) string {
	canonical := struct {
		Weekday  int     `json:"weekday"`
		Sections []int   `json:"sections"`
		Weeks    []int   `json:"weeks"`
		Location *string `json:"location"`
		Teacher  *string `json:"teacher"`
		Date     *string `json:"date"`
	}{v.Weekday, v.Sections, v.Weeks, v.Location, v.Teacher, v.Date}
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func courseMatchKey(name string, code, classCode, credits *string) string {
	return fmt.Sprintf("%s|%s|%s|%s", name, derefOrEmpty(code), derefOrEmpty(classCode), derefOrEmpty(credits))
}

func derefOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func valuesFromPreview(ps *PreviewSession) Values {
	return Values{
		Weekday: ps.Weekday, Sections: ps.Sections, Weeks: ps.Weeks,
		Location: ps.Location, Teacher: ps.Teacher, Date: ps.Date,
	}
}

func changedFields(old, new Values) []string {
	var out []string
	if old.Weekday != new.Weekday {
		out = append(out, "weekday")
	}
	if !equalInts(old.Sections, new.Sections) {
		out = append(out, "sections")
	}
	if !equalInts(old.Weeks, new.Weeks) {
		out = append(out, "weeks")
	}
	if !equalPtr(old.Location, new.Location) {
		out = append(out, "location")
	}
	if !equalPtr(old.Teacher, new.Teacher) {
		out = append(out, "teacher")
	}
	if !equalPtr(old.Date, new.Date) {
		out = append(out, "date")
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
