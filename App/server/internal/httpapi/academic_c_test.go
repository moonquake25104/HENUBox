package httpapi

// Phase C direct-connection tests. Everything runs against a throwaway schema
// and an httptest school double; the login helper is a canned Node script, so
// no real browser and no real school traffic is involved. Values used here are
// synthetic end to end.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/httpapi/handlers"
	"kairos/server/internal/schoolconn"
	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

const cTermOK = `{"xn":"2026","xqM":"0","xqName":"第一学期","userCode":"u1"}`

// fakeHelperScript writes a Node helper stub with the given behaviour baked in.
func fakeHelperScript(t *testing.T, mode string) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available for the fake helper")
	}
	var body string
	switch mode {
	case "cookies":
		body = `
process.stdout.write(JSON.stringify({event:'started'}) + '\n');
process.stdout.write(JSON.stringify({event:'cookies', url:'https://xk.henu.edu.cn/frame/homes.action', cookies:[
  {name:'JSESSIONID', value:'synthetic-session', domain:'xk.henu.edu.cn', path:'/', secure:false, httpOnly:true}
]}) + '\n');
process.stdin.resume();
process.stdin.on('data', (d) => { if (String(d).trim() === 'ack') process.exit(0); });
process.stdin.on('end', () => process.exit(0));`
	case "slow":
		body = `
process.stdout.write(JSON.stringify({event:'started'}) + '\n');
process.stdin.resume();
process.stdin.on('data', (d) => { if (String(d).trim() === 'cancel') process.exit(0); });
process.stdin.on('end', () => process.exit(0));
setTimeout(() => process.exit(2), 5 * 60 * 1000);`
	case "fail":
		body = `
process.stdout.write(JSON.stringify({event:'started'}) + '\n');
process.stdout.write(JSON.stringify({event:'failed', message:'登录窗口已关闭'}) + '\n');
process.exit(0);`
	default:
		t.Fatalf("unknown fake helper mode %q", mode)
	}
	script := filepath.Join(t.TempDir(), "helper.mjs")
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return script
}

// cSchoolDouble is the httptest school: behaviour is switched per request via
// the fields the test mutates between phases.
type cSchoolDouble struct {
	Term     string
	Data     []byte
	TermMode string // ok | boom | login
	DataMode string // ok | boom | login | empty | badgbk | slow
}

func newCSchoolDouble() *cSchoolDouble {
	return &cSchoolDouble{Term: cTermOK, Data: []byte(academicFixture)}
}

func (s *cSchoolDouble) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "showYearTerm.action"):
			switch s.TermMode {
			case "boom":
				http.Error(w, "internal", http.StatusInternalServerError)
			case "login":
				w.Header().Set("Content-Type", "text/html;charset=UTF-8")
				_, _ = w.Write([]byte(`<html><form action="/cas/logon.action"><input name="randnumber"></form></html>`))
			default:
				w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
				_, _ = w.Write([]byte(s.Term))
			}
		case strings.HasSuffix(r.URL.Path, "data10319.jsp"):
			if r.URL.Query().Get("params") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			switch s.DataMode {
			case "boom":
				http.Error(w, "internal", http.StatusInternalServerError)
			case "login":
				w.Header().Set("Content-Type", "text/html;charset=UTF-8")
				_, _ = w.Write([]byte(`<html><form action="/cas/logon.action"><input name="randnumber"></form></html>`))
			case "empty":
				w.Header().Set("Content-Type", "text/html;charset=UTF-8")
				// Recognizable structure with zero course rows: the parser
				// must classify this as EMPTY_SOURCE, not garbage.
				_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>
<div>2026-2027学年第一学期&ensp;课程门数：0&ensp;</div>
<table><thead><tr><th>上课班级代码</th><th>上课班级名称</th><th>课程</th><th>总学时</th><th>学分</th>
<th>修读性质</th><th>任课教师</th><th>选课状态</th><th>是否跨学科</th><th>教材</th><th>上课时间地点</th><th>备注</th></tr></thead>
<tbody></tbody></table></body></html>`))
			case "badgbk":
				w.Header().Set("Content-Type", "text/html;charset=GBK")
				_, _ = w.Write([]byte{0xcf, 0xb5, 0xff, 0xfe, 0x80, 0x81}) // invalid GBK sequences
			case "slow":
				time.Sleep(400 * time.Millisecond)
				w.Header().Set("Content-Type", "text/html;charset=UTF-8")
				_, _ = w.Write(s.Data)
			default:
				w.Header().Set("Content-Type", "text/html;charset=UTF-8")
				_, _ = w.Write(s.Data)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// cTestEnv builds the router with a phase C connector wired to the synthetic
// school double. The helper starts as the fast cookie stub; tests replace it.
func cTestEnv(t *testing.T) (h http.Handler, q *store.Queries, conn *pgx.Conn, connector *handlers.HenuConnector, school *cSchoolDouble, token string) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pgConn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	schema := "httpapi_henu_c_" + hex.EncodeToString(raw[:])
	if _, err := pgConn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := pgConn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("search_path: %v", err)
	}
	if err := migrate.Up(ctx, pgConn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
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
		_, _ = pgConn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		_ = pgConn.Close(context.Background())
	})

	school = newCSchoolDouble()
	srv := school.server()
	t.Cleanup(srv.Close)

	tmp := t.TempDir()
	keyFile := filepath.Join(tmp, "session.key")
	edge := filepath.Join(tmp, "msedge.exe")
	if err := os.WriteFile(edge, []byte("stub"), 0o600); err != nil {
		t.Fatalf("stub edge: %v", err)
	}
	connector = &handlers.HenuConnector{
		BaseURL:          srv.URL,
		KeyFile:          keyFile,
		LocalInteractive: true,
		NewClient: func() (*schoolconn.Client, error) {
			return schoolconn.NewClient(srv.URL)
		},
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	q = store.New(pool)
	h = New(Options{
		Log: slog.New(slog.NewTextHandler(func() io.Writer {
			if os.Getenv("DEBUG_LOG") != "" {
				return os.Stderr
			}
			return io.Discard
		}(), nil)),
		Username:     "kairos",
		PasswordHash: string(hash),
		JWTSecret:    []byte(testSecret),
		JWTTTL:       time.Hour,
		DB:           pgConn,
		Store:        q,
		Pool:         pool,
		DataDir:      t.TempDir(),
		Henu:         connector,
	})
	token = loginToken(t, h)
	return h, q, pgConn, connector, school, token
}

// attachFakeHelper makes interactive login available with a canned Node helper
// instead of a real Edge window.
func attachFakeHelper(t *testing.T, connector *handlers.HenuConnector, mode string) {
	t.Helper()
	connector.Helper = &schoolconn.LoginManager{
		ScriptPath:  fakeHelperScript(t, mode),
		EdgePath:    filepath.Join(t.TempDir(), "msedge.exe"),
		LoginURL:    "https://xk.henu.edu.cn/cas/login.action",
		Origin:      "https://xk.henu.edu.cn",
		ProfileRoot: t.TempDir(),
		StderrDir:   t.TempDir(),
	}
	if err := os.WriteFile(connector.Helper.EdgePath, []byte("stub"), 0o600); err != nil {
		t.Fatalf("stub edge: %v", err)
	}
}

// seedConnection writes a CONNECTED row with a synthetic session envelope so
// sync tests do not need the interactive login step.
func seedConnection(t *testing.T, connector *handlers.HenuConnector, q *store.Queries, generation int64, userCode string) {
	t.Helper()
	key, err := schoolconn.LoadSessionKey(connector.KeyFile, true)
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"cookies": []map[string]any{{
			"name": "JSESSIONID", "value": "synthetic-session",
			"domain": "xk.henu.edu.cn", "path": "/", "secure": false, "httpOnly": true,
		}},
		"user_code": userCode,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	sealed, err := schoolconn.Seal(key, payload, schoolconn.SessionAAD("kairos", "henu", generation))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if err := q.UpsertHenuConnected(context.Background(), "kairos", generation, sealed,
		"2026", "0", "2026-2027学年第一学期", time.Now()); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
}

func waitForJobState(t *testing.T, h http.Handler, token, jobID string, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rec := doJSON(t, h, http.MethodGet, "/api/academic/henu/sync-jobs/"+jobID, nil, token)
		if rec.Code == http.StatusOK {
			var body map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body["state"] == want {
				return body
			}
			if body["state"] == "FAILED" && want != "FAILED" {
				t.Fatalf("job reached FAILED while waiting for %s: %s", want, rec.Body.String())
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("job never reached %s", want)
	return nil
}

func waitForLoginState(t *testing.T, h http.Handler, token, loginID string, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rec := doJSON(t, h, http.MethodGet, "/api/academic/henu/login-sessions/"+loginID, nil, token)
		if rec.Code == http.StatusOK {
			var body map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body["state"] == want {
				return body
			}
			switch body["state"] {
			case "FAILED", "CANCELLED", "EXPIRED":
				if want != body["state"] {
					t.Fatalf("login reached %s while waiting for %s: %s", body["state"], want, rec.Body.String())
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("login never reached %s", want)
	return nil
}

func TestHenuEndpointsUnavailableWithoutConnector(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)

	rec := doJSON(t, h, http.MethodPost, "/api/academic/henu/login-sessions",
		map[string]any{"idempotency_key": testKey("c-unavail-login")}, token)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "INTERACTIVE_LOGIN_UNAVAILABLE") {
		t.Fatalf("login-sessions status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs",
		map[string]any{"semester": "2026S2", "expected_aggregate_revision": 0,
			"idempotency_key": testKey("c-unavail-job")}, token)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "DIRECT_FETCH_UNAVAILABLE") {
		t.Fatalf("sync-jobs status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHenuLoginFlow(t *testing.T) {
	h, _, conn, connector, _, token := cTestEnv(t)
	attachFakeHelper(t, connector, "cookies")
	ctx := context.Background()

	key := testKey("c-login-1")
	rec := doJSON(t, h, http.MethodPost, "/api/academic/henu/login-sessions", map[string]any{"idempotency_key": key}, token)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("login POST = %d body=%s", rec.Code, rec.Body.String())
	}
	var first map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &first)
	if first["state"] != "WAITING_FOR_USER" {
		t.Fatalf("initial state = %v", first["state"])
	}
	// Same key replays the same task and never opens a second window.
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/login-sessions", map[string]any{"idempotency_key": key}, token)
	var replay map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &replay)
	if replay["login_id"] != first["login_id"] {
		t.Fatalf("same key produced another task: %s vs %s", first["login_id"], replay["login_id"])
	}

	waitForLoginState(t, h, token, first["login_id"].(string), "SUCCEEDED")

	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/connection", nil, token)
	var connView map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &connView)
	if connView["connection_state"] != "CONNECTED" {
		t.Fatalf("connection_state = %v body=%s", connView["connection_state"], rec.Body.String())
	}
	caps := connView["capabilities"].(map[string]any)
	if caps["interactive_login"] != true || caps["direct_fetch"] != true {
		t.Fatalf("capabilities = %v", caps)
	}
	term := connView["current_term"].(map[string]any)
	if term["upstream_year"] != "2026" || term["label"] != "2026-2027学年第一学期" {
		t.Fatalf("current_term = %v", term)
	}
	gen1 := int64(0)
	if err := conn.QueryRow(ctx, "SELECT generation FROM henu_connections WHERE owner='kairos'").Scan(&gen1); err != nil {
		t.Fatalf("read generation: %v", err)
	}

	// A successful re-authorization advances the generation.
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/login-sessions", map[string]any{"idempotency_key": testKey("c-login-2")}, token)
	var second map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	waitForLoginState(t, h, token, second["login_id"].(string), "SUCCEEDED")
	gen2 := int64(0)
	if err := conn.QueryRow(ctx, "SELECT generation FROM henu_connections WHERE owner='kairos'").Scan(&gen2); err != nil {
		t.Fatalf("read generation: %v", err)
	}
	if gen2 != gen1+1 {
		t.Fatalf("generation = %d after re-auth, want %d", gen2, gen1+1)
	}
}

func TestHenuLoginSubjectGuard(t *testing.T) {
	h, q, _, connector, _, token := cTestEnv(t)
	attachFakeHelper(t, connector, "cookies")
	// The stored session belongs to subject u2 while the school (fake double)
	// reports u1: re-authorization must fail without touching the old row.
	seedConnection(t, connector, q, 1, "u2")
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", configuredSettingsBody(0), token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT = %d", rec.Code)
	}
	rec := doJSON(t, h, http.MethodPost, "/api/academic/henu/login-sessions", map[string]any{"idempotency_key": testKey("c-login-guard")}, token)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	state := waitForLoginState(t, h, token, body["login_id"].(string), "FAILED")
	errObj := state["error"].(map[string]any)
	if errObj["code"] != "UPSTREAM_ACCOUNT_CHANGED" {
		t.Fatalf("guard error = %v", errObj)
	}
	// The old session survives untouched.
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/connection", nil, token)
	var conn map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &conn)
	if conn["connection_state"] != "CONNECTED" {
		t.Fatalf("connection state after guard = %v", conn["connection_state"])
	}
}

func TestHenuLoginCancel(t *testing.T) {
	h, _, _, connector, _, token := cTestEnv(t)
	attachFakeHelper(t, connector, "slow")

	rec := doJSON(t, h, http.MethodPost, "/api/academic/henu/login-sessions", map[string]any{"idempotency_key": testKey("c-login-cancel")}, token)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	loginID := body["login_id"].(string)

	rec = doJSON(t, h, http.MethodDelete, "/api/academic/henu/login-sessions/"+loginID, nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d body=%s", rec.Code, rec.Body.String())
	}
	waitForLoginState(t, h, token, loginID, "CANCELLED")
	// Idempotent DELETE on the terminal task stays 204.
	rec = doJSON(t, h, http.MethodDelete, "/api/academic/henu/login-sessions/"+loginID, nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("second DELETE = %d", rec.Code)
	}
}

func TestHenuSyncJobLifecycle(t *testing.T) {
	h, q, _, connector, school, token := cTestEnv(t)
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", configuredSettingsBody(0), token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT = %d body=%s", rec.Code, rec.Body.String())
	}
	seedConnection(t, connector, q, 1, "u1")
	_ = school

	key := testKey("c-sync-1")
	body := map[string]any{"semester": "2026S2", "expected_aggregate_revision": 1, "idempotency_key": key}
	rec := doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs", body, token)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("sync POST = %d body=%s", rec.Code, rec.Body.String())
	}
	var job map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	if job["state"] != "QUEUED" {
		t.Fatalf("initial state = %v", job["state"])
	}
	// Same key + same request replays the same job.
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs", body, token)
	var replay map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &replay)
	if replay["job_id"] != job["job_id"] {
		t.Fatalf("same key produced another job: %s vs %s", job["job_id"], replay["job_id"])
	}
	// Same key with a different request is a conflict.
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs",
		map[string]any{"semester": "2026S2", "expected_aggregate_revision": 2, "idempotency_key": key}, token)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "IDEMPOTENCY_CONFLICT") {
		t.Fatalf("different request status = %d body=%s", rec.Code, rec.Body.String())
	}

	done := waitForJobState(t, h, token, job["job_id"].(string), "SUCCEEDED")
	previewID := done["preview_id"].(string)

	// The preview carries real direct-fetch provenance and is committable.
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+previewID, nil, token)
	var preview map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &preview)
	if preview["acquisition"] != "DIRECT_FETCH" {
		t.Fatalf("preview acquisition = %v", preview["acquisition"])
	}
	if preview["source_obtained_at"] == nil {
		t.Fatal("source_obtained_at missing")
	}
	if preview["can_commit"] != true {
		t.Fatalf("can_commit = %v status=%v", preview["can_commit"], preview["status"])
	}

	// Until the user commits, the timetable keeps the old provenance (an empty
	// semester has none at all).
	rec = doJSON(t, h, http.MethodGet, "/api/academic/timetable?semester=2026S2", nil, token)
	var tt map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tt)
	if tt["acquisition"] != "NONE" {
		t.Fatalf("pre-commit timetable acquisition = %v", tt["acquisition"])
	}

	commit := map[string]any{
		"expected_aggregate_revision": 1,
		"idempotency_key":             testKey("c-sync-commit"),
		"acknowledged_warning_codes":  []string{},
	}
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+previewID+"/commit", commit, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", rec.Code, rec.Body.String())
	}

	// After commit the batch provenance is visible in the timetable.
	rec = doJSON(t, h, http.MethodGet, "/api/academic/timetable?semester=2026S2", nil, token)
	_ = json.Unmarshal(rec.Body.Bytes(), &tt)
	if tt["acquisition"] != "DIRECT_FETCH" {
		t.Fatalf("post-commit acquisition = %v", tt["acquisition"])
	}
	sync := tt["source_sync"].(map[string]any)
	if sync["data_fetched_at"] == nil || sync["last_successful_import_at"] == nil {
		t.Fatalf("source_sync = %v", sync)
	}
	if sync["state"] != "READY" {
		t.Fatalf("source_sync.state = %v", sync["state"])
	}

	// A stale revision is rejected before any task starts.
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs",
		map[string]any{"semester": "2026S2", "expected_aggregate_revision": 1,
			"idempotency_key": testKey("c-sync-stale")}, token)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "REVISION_CONFLICT") {
		t.Fatalf("stale sync status = %d body=%s", rec.Code, rec.Body.String())
	}

	// Disconnect keeps the committed timetable and clears only the session.
	rec = doJSON(t, h, http.MethodDelete, "/api/academic/henu/connection", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE connection = %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/academic/timetable?semester=2026S2", nil, token)
	_ = json.Unmarshal(rec.Body.Bytes(), &tt)
	if tt["acquisition"] != "DIRECT_FETCH" {
		t.Fatalf("post-disconnect acquisition = %v", tt["acquisition"])
	}
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs",
		map[string]any{"semester": "2026S2", "expected_aggregate_revision": 2,
			"idempotency_key": testKey("c-sync-after-disconnect")}, token)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "CONNECTION_REQUIRED") {
		t.Fatalf("post-disconnect sync status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/connection", nil, token)
	var conn map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &conn)
	if conn["connection_state"] != "DISCONNECTED" || conn["current_term"] != nil {
		t.Fatalf("connection after disconnect = %s", rec.Body.String())
	}
}

func TestHenuSyncJobFailureClassification(t *testing.T) {
	h, q, _, connector, school, token := cTestEnv(t)
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", configuredSettingsBody(0), token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT = %d", rec.Code)
	}
	seedConnection(t, connector, q, 1, "u1")
	ctx := context.Background()

	start := func(key string) string {
		rec := doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs",
			map[string]any{"semester": "2026S2", "expected_aggregate_revision": 1,
				"idempotency_key": testKey(key)}, token)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("sync POST = %d body=%s", rec.Code, rec.Body.String())
		}
		var job map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &job)
		return job["job_id"].(string)
	}
	readConnState := func() string {
		rec := doJSON(t, h, http.MethodGet, "/api/academic/henu/connection", nil, token)
		var conn map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &conn)
		return conn["connection_state"].(string)
	}
	resetConnection := func() {
		if err := q.SetHenuConnectionState(ctx, "kairos", "CONNECTED", "", "", time.Now()); err != nil {
			t.Fatalf("reset connection state: %v", err)
		}
	}

	// Auth page from the school: job fails, connection requires re-login and
	// further sync attempts are refused up front.
	school.DataMode = "login"
	failed := waitForJobState(t, h, token, start("c-fail-login"), "FAILED")
	errObj := failed["error"].(map[string]any)
	if errObj["code"] != "UPSTREAM_AUTH_REQUIRED" {
		t.Fatalf("login-page error = %v", errObj)
	}
	if state := readConnState(); state != "AUTH_REQUIRED" {
		t.Fatalf("connection state = %s", state)
	}
	rec := doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs",
		map[string]any{"semester": "2026S2", "expected_aggregate_revision": 1,
			"idempotency_key": testKey("c-fail-login-2")}, token)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "UPSTREAM_AUTH_REQUIRED") {
		t.Fatalf("auth-required sync status = %d body=%s", rec.Code, rec.Body.String())
	}
	resetConnection()

	// 5xx is a temporary failure; credentials stay for a retry.
	school.DataMode = "boom"
	failed = waitForJobState(t, h, token, start("c-fail-boom"), "FAILED")
	if failed["error"].(map[string]any)["code"] != "UPSTREAM_UNAVAILABLE" {
		t.Fatalf("boom error = %v", failed["error"])
	}
	if state := readConnState(); state != "TEMPORARY_FAILURE" {
		t.Fatalf("connection state after boom = %s", state)
	}
	resetConnection()

	// Empty table and broken encoding never touch the connection state.
	school.DataMode = "empty"
	failed = waitForJobState(t, h, token, start("c-fail-empty"), "FAILED")
	if failed["error"].(map[string]any)["code"] != "EMPTY_SOURCE" {
		t.Fatalf("empty error = %v", failed["error"])
	}
	if state := readConnState(); state != "CONNECTED" {
		t.Fatalf("connection state after empty = %s", state)
	}
	school.DataMode = "badgbk"
	failed = waitForJobState(t, h, token, start("c-fail-badgbk"), "FAILED")
	if failed["error"].(map[string]any)["code"] != "INVALID_ENCODING" {
		t.Fatalf("badgbk error = %v", failed["error"])
	}

	// Term mismatch: school term label vs configured semester label.
	school.DataMode = "ok"
	school.Term = `{"xn":"2026","xqM":"1","xqName":"第二学期","userCode":"u1"}`
	failed = waitForJobState(t, h, token, start("c-fail-term"), "FAILED")
	if failed["error"].(map[string]any)["code"] != "UPSTREAM_TERM_MISMATCH" {
		t.Fatalf("term error = %v", failed["error"])
	}
}

func TestHenuSyncMidFlightRevisionConflict(t *testing.T) {
	h, q, conn, connector, school, token := cTestEnv(t)
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", configuredSettingsBody(0), token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT = %d", rec.Code)
	}
	seedConnection(t, connector, q, 1, "u1")
	school.DataMode = "slow"

	rec := doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs",
		map[string]any{"semester": "2026S2", "expected_aggregate_revision": 1,
			"idempotency_key": testKey("c-conflict-job")}, token)
	var job map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &job)

	// A concurrent active job blocks another key.
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/sync-jobs",
		map[string]any{"semester": "2026S2", "expected_aggregate_revision": 1,
			"idempotency_key": testKey("c-conflict-second")}, token)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "SYNC_IN_PROGRESS") {
		t.Fatalf("second job status = %d body=%s", rec.Code, rec.Body.String())
	}

	// A legacy write during the fetch invalidates the result.
	createLegacyCourse(t, h, token)

	failed := waitForJobState(t, h, token, job["job_id"].(string), "FAILED")
	if failed["error"].(map[string]any)["code"] != "REVISION_CONFLICT" {
		t.Fatalf("conflict error = %v", failed["error"])
	}
	// No preview was persisted by the stale fetch.
	var count int
	if err := conn.QueryRow(context.Background(), "SELECT count(*) FROM academic_previews").Scan(&count); err != nil {
		t.Fatalf("count previews: %v", err)
	}
	if count != 0 {
		t.Fatalf("stale result persisted %d previews", count)
	}
}

func TestHenuRecoverInterrupted(t *testing.T) {
	h, q, conn, connector, _, token := cTestEnv(t)
	attachFakeHelper(t, connector, "cookies")
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", configuredSettingsBody(0), token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT = %d", rec.Code)
	}
	seedConnection(t, connector, q, 1, "u1")
	ctx := context.Background()

	// Simulate a crash: non-terminal rows plus a leftover profile directory.
	loginID := "11111111-1111-4111-8111-111111111111"
	jobID := "22222222-2222-4222-8222-222222222222"
	if _, err := conn.Exec(ctx, `
		INSERT INTO henu_login_sessions (id, owner, generation, state, idempotency_key, created_at, expires_at)
		VALUES ($1::uuid, 'kairos', 1, 'WAITING_FOR_USER', 'crash-login', now(), now() + interval '10 minutes')`, loginID); err != nil {
		t.Fatalf("seed login: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO henu_sync_jobs (id, owner, semester, generation, expected_aggregate_revision, idempotency_key, request_sha256, state, created_at)
		VALUES ($1::uuid, 'kairos', '2026S2', 1, 0, 'crash-job', 'sha', 'RUNNING', now())`, jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	profileDir := filepath.Join(connector.Helper.ProfileRoot, loginID)
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	connector.RecoverInterrupted(ctx, q)

	var state string
	if err := conn.QueryRow(ctx, "SELECT state FROM henu_login_sessions WHERE id=$1::uuid", loginID).Scan(&state); err != nil {
		t.Fatalf("read login: %v", err)
	}
	if state != "FAILED" {
		t.Fatalf("login state after recovery = %s", state)
	}
	var errCode string
	if err := conn.QueryRow(ctx, "SELECT error_code FROM henu_sync_jobs WHERE id=$1::uuid", jobID).Scan(&errCode); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if errCode != "JOB_INTERRUPTED" {
		t.Fatalf("job error after recovery = %s", errCode)
	}
	if _, statErr := os.Stat(profileDir); !os.IsNotExist(statErr) {
		t.Fatalf("profile directory survived recovery: %v", statErr)
	}
}

func TestHenuTimetableAcquisitionForFileImport(t *testing.T) {
	h, _, _, connector, _, token := cTestEnv(t)
	_ = connector
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", configuredSettingsBody(0), token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT = %d", rec.Code)
	}
	// File import keeps the classic provenance: FILE_IMPORT with no fetch time.
	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	var preview map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &preview)
	if preview["acquisition"] != "FILE_IMPORT" {
		t.Fatalf("file preview acquisition = %v", preview["acquisition"])
	}
	commit := map[string]any{
		"expected_aggregate_revision": 1,
		"idempotency_key":             testKey("c-file-commit"),
		"acknowledged_warning_codes":  []string{},
	}
	rec = doJSON(t, h, http.MethodPost, fmt.Sprintf("/api/academic/henu/previews/%s/commit", preview["preview_id"]), commit, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/api/academic/timetable?semester=2026S2", nil, token)
	var tt map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tt)
	if tt["acquisition"] != "FILE_IMPORT" {
		t.Fatalf("timetable acquisition = %v", tt["acquisition"])
	}
	sync := tt["source_sync"].(map[string]any)
	if sync["data_fetched_at"] != nil {
		t.Fatalf("file import must not fabricate a fetch time: %v", sync["data_fetched_at"])
	}
	if sync["last_checked_at"] != nil {
		t.Fatalf("file import must not fabricate a check time: %v", sync["last_checked_at"])
	}
}
