package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const b2SessionUUID = "2b2b2b2b-0000-4000-8000-000000000004"
const b2CourseUUID = "2b2b2b2b-0000-4000-8000-000000000001"

func b2Fixture(weekday string, weeks string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>
<div>2026-2027学年第一学期&ensp;课程门数：1&ensp;</div>
<table><thead><tr>
<th>上课班级代码</th><th>上课班级名称</th><th>课程</th><th>总学时</th><th>学分</th>
<th>修读性质</th><th>任课教师</th><th>选课状态</th><th>是否跨学科</th><th>教材</th><th>上课时间地点</th><th>备注</th>
</tr></thead><tbody>
<tr><td>D-1</td><td>示例班</td><td>[D-1]合成来源课</td><td>54</td><td>3.0</td><td>初修</td><td>示例教师</td>
<td>选中</td><td>否</td><td>否</td><td>` + weeks + ` ` + weekday + `[11-13] 示例楼101(60)</td><td></td></tr>
</tbody></table></body></html>`
}

func seedB2Committed(t *testing.T, q interface {
	InsertAcademicCourse(ctx context.Context, id, owner, semester, school, name string, courseCode, classCode, credits *string, batchID *string) error
	InsertSourceEntry(ctx context.Context, id, owner, semester, school, courseID string, upstreamKey *string) error
	InsertSourceRevision(ctx context.Context, id, entryID string, batchID *string, sha string, values []byte) error
	InsertAcademicSession(ctx context.Context, id, courseID, entryID string) error
	UpsertLegacyLink(ctx context.Context, sessionID string, courseID int64) error
}, courseID int64) {
	t.Helper()
	ctx := context.Background()
	if err := q.InsertAcademicCourse(ctx, b2CourseUUID, "kairos", "2026S2", "henu", "合成来源课", strPtrH("D-1"), strPtrH("D-1"), strPtrH("3.0"), nil); err != nil {
		t.Fatalf("course: %v", err)
	}
	if err := q.InsertSourceEntry(ctx, "2b2b2b2b-0000-4000-8000-000000000002", "kairos", "2026S2", "henu", b2CourseUUID, strPtrH("D-1")); err != nil {
		t.Fatalf("entry: %v", err)
	}
	values := map[string]any{
		"name": "合成来源课", "teacher": "示例教师", "location": "示例楼101", "color": nil,
		"hidden": false, "weekday": 3, "sections": []int{11, 12, 13},
		"weeks": []int{1, 2, 3}, "date": nil, "start_time": "18:10", "end_time": "20:40",
	}
	raw, _ := json.Marshal(values)
	if err := q.InsertSourceRevision(ctx, "2b2b2b2b-0000-4000-8000-000000000003", "2b2b2b2b-0000-4000-8000-000000000002", nil, "sha-b2", raw); err != nil {
		t.Fatalf("revision: %v", err)
	}
	if err := q.InsertAcademicSession(ctx, b2SessionUUID, b2CourseUUID, "2b2b2b2b-0000-4000-8000-000000000002"); err != nil {
		t.Fatalf("session: %v", err)
	}
	if courseID > 0 {
		if err := q.UpsertLegacyLink(ctx, b2SessionUUID, courseID); err != nil {
			t.Fatalf("link: %v", err)
		}
	}
}

type previewResponse struct {
	PreviewID string `json:"preview_id"`
	Status    string `json:"status"`
	CanCommit bool   `json:"can_commit"`
	ExpiresAt string `json:"expires_at"`
	Summary   struct {
		LogicalCourses int `json:"logical_courses"`
		Sessions       int `json:"sessions"`
	} `json:"summary"`
	Issues []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
	} `json:"issues"`
	Courses []struct {
		PreviewCourseID  string  `json:"preview_course_id"`
		AcademicCourseID *string `json:"academic_course_id"`
		Sessions         []struct {
			PreviewSessionID string  `json:"preview_session_id"`
			SessionID        *string `json:"session_id"`
		} `json:"sessions"`
	} `json:"courses"`
	Diff struct {
		Added     []string `json:"added"`
		Unchanged []string `json:"unchanged"`
		Changed   []struct {
			PreviewSessionID string   `json:"preview_session_id"`
			SessionID        string   `json:"session_id"`
			ChangedFields    []string `json:"changed_fields"`
		} `json:"changed"`
		Missing []struct {
			SessionID string  `json:"session_id"`
			Decision  *string `json:"decision"`
		} `json:"missing"`
		Ambiguous []struct {
			PreviewSessionID    string   `json:"preview_session_id"`
			CandidateSessionIDs []string `json:"candidate_session_ids"`
		} `json:"ambiguous"`
	} `json:"diff"`
}

func decodePreview(t *testing.T, body []byte) previewResponse {
	t.Helper()
	var p previewResponse
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	return p
}

func TestRebaseRetainMissingSession(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)
	seedB2Committed(t, q, 0)

	// Different weeks: the committed arrangement is missing and the uploaded one
	// is new; without a decision the preview must not be READY.
	rec := multipartPreview(t, h, token, "2026S2", []byte(b2Fixture("三", "5-6周")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if preview.Status != "NEEDS_REVIEW" || len(preview.Diff.Missing) != 1 || preview.Diff.Missing[0].Decision != nil {
		t.Fatalf("preview = %s missing=%+v", preview.Status, preview.Diff.Missing)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/rebase", map[string]any{
		"expected_aggregate_revision": 1,
		"resolutions": []map[string]any{{
			"kind":       "RETAIN_SESSION",
			"session_id": b2SessionUUID,
		}},
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rebase = %d body=%s", rec.Code, rec.Body.String())
	}
	rebased := decodePreview(t, rec.Body.Bytes())
	if rebased.Status != "READY" {
		t.Fatalf("rebased status = %s", rebased.Status)
	}
	if len(rebased.Diff.Missing) != 1 || rebased.Diff.Missing[0].Decision == nil || *rebased.Diff.Missing[0].Decision != "RETAIN" {
		t.Fatalf("missing decision = %+v", rebased.Diff.Missing)
	}
}

func TestPreviewDiffAndRebaseLink(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	// Seed a committed source arrangement without a legacy projection.
	seedB2Committed(t, q, 0)

	// Same content: unchanged, no added, READY.
	rec := multipartPreview(t, h, token, "2026S2", []byte(b2Fixture("三", "1-3周")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first preview = %d body=%s", rec.Code, rec.Body.String())
	}
	first := decodePreview(t, rec.Body.Bytes())
	if first.Status != "READY" || !first.CanCommit {
		t.Fatalf("same content status=%s can_commit=%v", first.Status, first.CanCommit)
	}
	if len(first.Diff.Unchanged) != 1 || len(first.Diff.Added) != 0 || len(first.Diff.Missing) != 0 {
		t.Fatalf("diff = %+v", first.Diff)
	}
	if first.Courses[0].AcademicCourseID == nil || first.Courses[0].Sessions[0].SessionID == nil {
		t.Fatal("matched ids missing")
	}

	// Changed weekday without stable session key: must be ambiguous/missing,
	// not silently added.
	rec = multipartPreview(t, h, token, "2026S2", []byte(b2Fixture("四", "1-3周")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("second preview = %d body=%s", rec.Code, rec.Body.String())
	}
	second := decodePreview(t, rec.Body.Bytes())
	if second.Status != "NEEDS_REVIEW" {
		t.Fatalf("changed weekday status = %s", second.Status)
	}
	if len(second.Diff.Added) != 1 || len(second.Diff.Missing) != 1 {
		t.Fatalf("second diff = %+v", second.Diff)
	}
	newPS := second.Courses[0].Sessions[0].PreviewSessionID

	// Rebase with LINK_SESSION turns it into a real "changed" row.
	rebaseBody := map[string]any{
		"expected_aggregate_revision": 1,
		"confirm_semester_mapping":    false,
		"resolutions": []map[string]any{{
			"kind":               "LINK_SESSION",
			"preview_session_id": newPS,
			"session_id":         b2SessionUUID,
		}},
	}
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+second.PreviewID+"/rebase", rebaseBody, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rebase = %d body=%s", rec.Code, rec.Body.String())
	}
	third := decodePreview(t, rec.Body.Bytes())
	if third.Status != "READY" || !third.CanCommit {
		t.Fatalf("rebased status=%s can_commit=%v", third.Status, third.CanCommit)
	}
	if len(third.Diff.Changed) != 1 || len(third.Diff.Missing) != 0 {
		t.Fatalf("rebased diff = %+v", third.Diff)
	}
	if !strings.Contains(strings.Join(third.Diff.Changed[0].ChangedFields, ","), "weekday") {
		t.Fatalf("changed fields = %v", third.Diff.Changed[0].ChangedFields)
	}
	if third.ExpiresAt != second.ExpiresAt {
		t.Fatalf("rebase must keep the original deadline: %s vs %s", third.ExpiresAt, second.ExpiresAt)
	}
	if third.PreviewID == second.PreviewID {
		t.Fatal("rebase must create a new preview id")
	}

	// The source preview is immutable.
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+second.PreviewID, nil, token)
	var old previewResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &old)
	if len(old.Diff.Added) != 1 || len(old.Diff.Changed) != 0 {
		t.Fatalf("source preview changed: %+v", old.Diff)
	}
}
