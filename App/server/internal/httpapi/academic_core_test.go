package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

// testKey derives a deterministic canonical UUID from a readable seed so tests
// can keep meaningful names while satisfying the UUID-only idempotency contract.
func testKey(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	h := hex.EncodeToString(sum[:16])
	return h[0:8] + "-" + h[8:12] + "-4" + h[13:16] + "-a" + h[17:20] + "-" + h[20:32]
}

func academicTestHandler(t *testing.T) (http.Handler, *store.Queries) {
	h, q, _, _, _ := academicTestEnv(t)
	return h, q
}

// academicTestEnv additionally exposes the schema connection and a small
// connection pool so tests can inject DDL or issue concurrent requests.
func academicTestEnv(t *testing.T) (http.Handler, *store.Queries, *pgx.Conn, *pgxpool.Pool, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	schema := "httpapi_academic_" + hex.EncodeToString(b[:])
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("search_path: %v", err)
	}
	if err := migrate.Up(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Pool connections need the same search_path as the schema connection.
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.MaxConns = 4
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+schema)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		_ = conn.Close(context.Background())
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	logWriter := io.Writer(io.Discard)
	if os.Getenv("DEBUG_LOG") != "" {
		logWriter = os.Stderr
	}
	q := store.New(pool)
	h := New(Options{
		Log:          slog.New(slog.NewTextHandler(logWriter, nil)),
		Username:     "kairos",
		PasswordHash: string(hash),
		JWTSecret:    []byte(testSecret),
		JWTTTL:       time.Hour,
		DB:           conn,
		Store:        q,
		Pool:         pool,
		DataDir:      t.TempDir(),
	})
	return h, q, conn, pool, schema
}

func settingsBody(revision int64, times bool) map[string]any {
	periods := make([]map[string]any, 0, 13)
	for i := 1; i <= 13; i++ {
		p := map[string]any{"index": i, "start_time": nil, "end_time": nil, "configured": false}
		if times {
			start := fmt.Sprintf("%02d:%02d", 8+(i-1)/2, 0)
			end := fmt.Sprintf("%02d:%02d", 8+(i-1)/2, 45)
			p = map[string]any{"index": i, "start_time": start, "end_time": end, "configured": true}
		}
		periods = append(periods, p)
	}
	return map[string]any{
		"semester":          "2026S2",
		"expected_revision": revision,
		"semester_label":    "2026-2027学年第一学期",
		"week_anchor":       map[string]any{"start_date": "2026-08-31", "source": "USER", "confirmed": true},
		"total_weeks":       18,
		"period_count":      13,
		"periods":           periods,
	}
}

func TestAcademicSettingsEndpoints(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)

	rec := doJSON(t, h, http.MethodGet, "/api/academic/semester-settings?semester=2026S2", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET default status = %d body=%s", rec.Code, rec.Body.String())
	}
	var def map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &def)
	if def["revision"].(float64) != 0 {
		t.Fatalf("default revision = %v", def["revision"])
	}
	if n := len(def["periods"].([]any)); n != 13 {
		t.Fatalf("default periods = %d", n)
	}

	rec = doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", settingsBody(0, false), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	var saved map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &saved)
	if saved["revision"].(float64) != 1 {
		t.Fatalf("saved revision = %v", saved["revision"])
	}
	if saved["week_anchor"].(map[string]any)["start_date"] != "2026-08-31" {
		t.Fatalf("anchor = %v", saved["week_anchor"])
	}

	rec = doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", settingsBody(0, false), token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	var conflict map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &conflict)
	if conflict["code"] != "REVISION_CONFLICT" {
		t.Fatalf("stale PUT code = %v", conflict["code"])
	}

	missing := settingsBody(1, false)
	delete(missing, "expected_revision")
	rec = doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", missing, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing expected_revision status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAcademicConnectionCapabilities(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)

	rec := doJSON(t, h, http.MethodGet, "/api/academic/henu/connection", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		ConnectionState string          `json:"connection_state"`
		Capabilities    map[string]bool `json:"capabilities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ConnectionState != "UNAVAILABLE" {
		t.Fatalf("state = %s", body.ConnectionState)
	}
	if !body.Capabilities["file_import"] || body.Capabilities["direct_fetch"] || body.Capabilities["interactive_login"] {
		t.Fatalf("capabilities = %+v", body.Capabilities)
	}
}

const academicFixture = `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>
<div>2026-2027学年第一学期&ensp;课程门数：2&ensp;</div>
<table><thead><tr><th>上课班级代码</th><th>上课班级名称</th><th>课程</th><th>总学时</th><th>学分</th>
<th>修读性质</th><th>任课教师</th><th>选课状态</th><th>是否跨学科</th><th>教材</th><th>上课时间地点</th><th>备注</th></tr></thead>
<tbody>
<tr><td>D-1</td><td>示例班</td><td>[D-1]合成课程甲</td><td>54</td><td>3.0</td><td>初修</td><td>[T-1]示例教师</td>
<td>选中</td><td>否</td><td>否</td><td>1-18周 三[11-13] 示例楼306(120)</td><td></td></tr>
<tr><td>D-2</td><td>示例班</td><td>[D-2]合成课程乙</td><td>36</td><td>2.0</td><td>初修</td><td>[T-2]示例教师</td>
<td>选中</td><td>否</td><td>否</td><td>4-14周(双) 四[11-13] 示例楼307(90)</td><td></td></tr>
</tbody></table></body></html>`

func multipartPreview(t *testing.T, h http.Handler, token, semester string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("semester", semester); err != nil {
		t.Fatalf("write field: %v", err)
	}
	fw, err := w.CreateFormFile("file", "timetable.xls")
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/academic/henu/previews", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAcademicPreviewLifecycle(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()

	before, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil {
		t.Fatalf("list courses: %v", err)
	}
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", settingsBody(0, false), token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview status = %d body=%s", rec.Code, rec.Body.String())
	}
	var preview struct {
		PreviewID string `json:"preview_id"`
		Status    string `json:"status"`
		CanCommit bool   `json:"can_commit"`
		Summary   struct {
			LogicalCourses int `json:"logical_courses"`
			Sessions       int `json:"sessions"`
		} `json:"summary"`
		Courses []struct {
			Sessions []struct {
				StartTime *string `json:"start_time"`
			} `json:"sessions"`
		} `json:"courses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.Summary.LogicalCourses != 2 || preview.Summary.Sessions != 2 {
		t.Fatalf("summary = %+v", preview.Summary)
	}
	if preview.Status != "NEEDS_CONFIGURATION" || preview.CanCommit {
		t.Fatalf("status = %s can_commit=%v", preview.Status, preview.CanCommit)
	}
	if preview.Courses[0].Sessions[0].StartTime != nil {
		t.Fatal("start_time must stay null without configured periods")
	}

	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+preview.PreviewID, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET preview status = %d body=%s", rec.Code, rec.Body.String())
	}
	after, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil {
		t.Fatalf("list courses after: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("courses changed from %d to %d", len(before), len(after))
	}

	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/99999999-9999-4999-8999-999999999999", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown preview status = %d", rec.Code)
	}

	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", settingsBody(1, false), token); rec.Code != http.StatusOK {
		t.Fatalf("second settings PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+preview.PreviewID, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET stale preview status = %d body=%s", rec.Code, rec.Body.String())
	}
	var stale struct {
		CanCommit  bool `json:"can_commit"`
		Validation struct {
			State                    string `json:"state"`
			CurrentAggregateRevision int64  `json:"current_aggregate_revision"`
		} `json:"validation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &stale); err != nil {
		t.Fatalf("decode stale preview: %v", err)
	}
	if stale.Validation.State != "STALE" || stale.CanCommit {
		t.Fatalf("validation = %+v can_commit=%v", stale.Validation, stale.CanCommit)
	}
}

func TestAcademicPreviewRejectsBadUploads(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)

	ole := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, []byte("binary")...)
	rec := multipartPreview(t, h, token, "2026S2", ole)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("OLE status = %d body=%s", rec.Code, rec.Body.String())
	}

	login := []byte(`<html><body><div>统一身份认证</div><div>用户登录</div><p>请输入密码和验证码</p></body></html>`)
	rec = multipartPreview(t, h, token, "2026S2", login)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "UPSTREAM_AUTH_REQUIRED") {
		t.Fatalf("login page status = %d body=%s", rec.Code, rec.Body.String())
	}

	unknown := []byte(`<html><body><table><tr><td>通知</td></tr></table></body></html>`)
	rec = multipartPreview(t, h, token, "2026S2", unknown)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "UNRECOGNIZED_DOCUMENT") {
		t.Fatalf("unknown doc status = %d body=%s", rec.Code, rec.Body.String())
	}

	big := bytes.Repeat([]byte("a"), 8<<20+1)
	rec = multipartPreview(t, h, token, "2026S2", big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestAcademicExpiredPreviewTombstone covers 410 -> (retention over) -> 404.
func TestAcademicExpiredPreviewTombstone(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	now := time.Now()

	id := "55555555-5555-4555-8555-555555555555"
	row := store.AcademicPreviewRow{
		ID: id, Owner: "kairos", Semester: "2026S2", Source: "henu", Acquisition: "FILE_IMPORT",
		ParserVersion: "henu-html-v1", Status: "NEEDS_CONFIGURATION",
		Payload: []byte(`{"schema_version":1}`), Summary: []byte(`{}`), Issues: []byte(`[]`),
		FileSHA256: "x", FileBytes: 1,
		ReceivedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour),
	}
	if err := q.InsertAcademicPreview(ctx, row); err != nil {
		t.Fatalf("insert expired preview: %v", err)
	}

	rec := doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+id, nil, token)
	if rec.Code != http.StatusGone {
		t.Fatalf("first GET = %d, want 410; body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+id, nil, token)
	if rec.Code != http.StatusGone {
		t.Fatalf("second GET = %d, want 410", rec.Code)
	}
	if _, err := q.DeleteExpiredTombstones(ctx, now.Add(31*24*time.Hour), 10); err != nil {
		t.Fatalf("purge tombstones: %v", err)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+id, nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET after purge = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}
