package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

type recordingRecomputer struct {
	mu      sync.Mutex
	exams   int
	tasks   int
	courses int
	ai      int
}

func (r *recordingRecomputer) RecomputeExams(context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exams++
}

func (r *recordingRecomputer) RecomputeTasks(context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks++
}

func (r *recordingRecomputer) RecomputeCourses(context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.courses++
}

func (r *recordingRecomputer) RecomputeAI(context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ai++
}

func (r *recordingRecomputer) courseCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.courses
}

func (r *recordingRecomputer) examCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exams
}

func newNotifyTestRouter(t *testing.T) (http.Handler, string, *recordingRecomputer) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	schema := "notify_httpapi_test_" + randomHex()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("set search_path to %s: %v", schema, err)
	}
	if err := migrate.Up(ctx, conn); err != nil {
		t.Fatalf("apply migrations to schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		defer conn.Close(context.Background())
		if _, err := conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	fake := &recordingRecomputer{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Options{
		Log:             log,
		Username:        "kairos",
		PasswordHash:    string(hash),
		JWTSecret:       []byte(testSecret),
		JWTTTL:          time.Hour,
		Store:           store.New(conn),
		DataDir:         t.TempDir(),
		NotifyScheduler: fake,
	})
	return h, loginToken(t, h), fake
}

func getNotifyConfig(t *testing.T, h http.Handler, token string) dto.NotificationConfig {
	t.Helper()
	rec := doJSON(t, h, http.MethodGet, "/api/notify/config", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var cfg dto.NotificationConfig
	decodeBody(t, rec, &cfg)
	return cfg
}

func examOffsets(t *testing.T, cfg dto.NotificationConfig) []int {
	t.Helper()
	var offsets []int
	if err := json.Unmarshal([]byte(cfg.ExamOffsetsJSON), &offsets); err != nil {
		t.Fatalf("exam_offsets_json is not a JSON int array: %q: %v", cfg.ExamOffsetsJSON, err)
	}
	return offsets
}

func TestNotifyConfigGetDefaults(t *testing.T) {
	h, token, _ := newNotifyTestRouter(t)

	cfg := getNotifyConfig(t, h, token)
	if !cfg.Enabled {
		t.Fatalf("default enabled should be true: %+v", cfg)
	}
	if got := examOffsets(t, cfg); !reflect.DeepEqual(got, []int{1440, 60}) {
		t.Fatalf("default offsets = %v, want [1440 60]", got)
	}
	if cfg.AndroidChannelCreated {
		t.Fatal("android_channel_created must always be false")
	}
	if cfg.CreatedAt == "" || cfg.UpdatedAt == "" {
		t.Fatalf("timestamps should be populated: %+v", cfg)
	}
}

func TestNotifyConfigPatchMergesAndRecomputes(t *testing.T) {
	h, token, fake := newNotifyTestRouter(t)

	enabled := false
	offsets := "[30,10]"
	rec := doJSON(t, h, http.MethodPatch, "/api/notify/config", dto.UpdateNotificationConfigRequest{
		Enabled:         &enabled,
		ExamOffsetsJSON: &offsets,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var patched dto.NotificationConfig
	decodeBody(t, rec, &patched)
	if patched.Enabled {
		t.Fatalf("enabled should be false after patch: %+v", patched)
	}
	if got := examOffsets(t, patched); !reflect.DeepEqual(got, []int{30, 10}) {
		t.Fatalf("patched offsets = %v, want [30 10]", got)
	}

	cfg := getNotifyConfig(t, h, token)
	if cfg.Enabled {
		t.Fatalf("GET should reflect enabled=false: %+v", cfg)
	}
	if got := examOffsets(t, cfg); !reflect.DeepEqual(got, []int{30, 10}) {
		t.Fatalf("GET offsets = %v, want [30 10]", got)
	}
	if calls := fake.examCalls(); calls != 1 {
		t.Fatalf("RecomputeExams calls = %d, want 1", calls)
	}
}

func TestNotifyConfigPatchInvalidOffsets(t *testing.T) {
	h, token, fake := newNotifyTestRouter(t)

	bad := "not json"
	rec := doJSON(t, h, http.MethodPatch, "/api/notify/config", dto.UpdateNotificationConfigRequest{
		ExamOffsetsJSON: &bad,
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid offsets status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if calls := fake.examCalls(); calls != 0 {
		t.Fatalf("RecomputeExams calls = %d, want 0 on invalid input", calls)
	}

	cfg := getNotifyConfig(t, h, token)
	if got := examOffsets(t, cfg); !reflect.DeepEqual(got, []int{1440, 60}) {
		t.Fatalf("offsets changed after rejected patch: %v", got)
	}
}

func TestNotifyConfigPatchNonPositiveOffsets(t *testing.T) {
	h, token, fake := newNotifyTestRouter(t)

	bad := "[0,60]"
	rec := doJSON(t, h, http.MethodPatch, "/api/notify/config", dto.UpdateNotificationConfigRequest{
		ExamOffsetsJSON: &bad,
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-positive offsets status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if calls := fake.examCalls(); calls != 0 {
		t.Fatalf("RecomputeExams calls = %d, want 0 on invalid input", calls)
	}
}

func TestNotifyConfigPatchEnabledOnlyKeepsOffsets(t *testing.T) {
	h, token, fake := newNotifyTestRouter(t)

	enabled := true
	rec := doJSON(t, h, http.MethodPatch, "/api/notify/config", dto.UpdateNotificationConfigRequest{
		Enabled: &enabled,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var patched dto.NotificationConfig
	decodeBody(t, rec, &patched)
	if !patched.Enabled {
		t.Fatalf("enabled should stay true: %+v", patched)
	}
	if got := examOffsets(t, patched); !reflect.DeepEqual(got, []int{1440, 60}) {
		t.Fatalf("offsets should be unchanged, got %v", got)
	}
	if calls := fake.examCalls(); calls != 1 {
		t.Fatalf("RecomputeExams calls = %d, want 1", calls)
	}
}
