package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// fakeWebDAV is a stateful in-memory WebDAV stub with ETag support.
type fakeWebDAV struct {
	snapshot     string
	snapshotEtag string
	aiBlob       string
	aiEtag       string
	// failUploadOnce makes the next snapshot PUT return 412.
	failUploadOnce bool
	uploadCount    int
}

func (f *fakeWebDAV) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/kairos-sync.json" && r.Method == http.MethodGet:
			if f.snapshot == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("ETag", f.snapshotEtag)
			w.Write([]byte(f.snapshot))
		case r.URL.Path == "/kairos-sync.json" && r.Method == http.MethodPut:
			f.uploadCount++
			if f.failUploadOnce {
				f.failUploadOnce = false
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			f.snapshot = string(buf)
			f.snapshotEtag = `"snap-` + itoa(f.uploadCount) + `"`
			w.Header().Set("ETag", f.snapshotEtag)
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/kairos-sync.json" && r.Method == http.MethodHead:
			if f.snapshot == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/kairos-ai-settings.enc" && r.Method == http.MethodGet:
			if f.aiBlob == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("ETag", f.aiEtag)
			w.Write([]byte(f.aiBlob))
		case r.URL.Path == "/kairos-ai-settings.enc" && r.Method == http.MethodPut:
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			f.aiBlob = string(buf)
			f.aiEtag = `"ai-1"`
			w.Header().Set("ETag", f.aiEtag)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func newTestService(t *testing.T, q *store.Queries, conn *pgx.Conn, dataDir string) *Service {
	t.Helper()
	return &Service{Q: q, Pool: conn, DataDir: dataDir}
}

func configureSync(t *testing.T, q *store.Queries, serverURL, password string) {
	t.Helper()
	ctx := context.Background()
	if err := q.UpdateSyncConfigCredentials(ctx, store.UpdateSyncConfigCredentialsParams{
		ServerUrl: serverURL,
		Username:  "user",
		Password:  password,
		AutoSync:  false,
	}); err != nil {
		t.Fatalf("configure sync: %v", err)
	}
}

func TestSyncNowUploadsLocalOnly(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	// Seed a local task.
	if _, err := q.CreateTask(ctx, store.CreateTaskParams{
		SyncID:      "task-local-1",
		Title:       "Local Task",
		Description: "",
		Status:      "todo",
		Priority:    "medium",
		Tags:        []byte("[]"),
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	fake := &fakeWebDAV{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	configureSync(t, q, srv.URL, "pass")

	svc := newTestService(t, q, conn, t.TempDir())
	result, err := svc.SyncNow(ctx)
	if err != nil {
		t.Fatalf("sync now: %v", err)
	}
	if !result.Uploaded || result.Downloaded {
		t.Fatalf("result = %+v, want uploaded=true downloaded=false", result)
	}
	if fake.snapshot == "" {
		t.Fatal("snapshot should have been uploaded")
	}
	var uploaded SyncData
	if err := json.Unmarshal([]byte(fake.snapshot), &uploaded); err != nil {
		t.Fatalf("parse uploaded snapshot: %v", err)
	}
	if len(uploaded.Tasks) != 1 || uploaded.Tasks[0].Title != "Local Task" {
		t.Fatalf("uploaded tasks = %+v", uploaded.Tasks)
	}

	cfg, err := q.GetSyncConfig(ctx)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if !cfg.LastSyncAt.Valid {
		t.Fatal("last_sync_at should be set after sync")
	}
	if !cfg.RemoteEtag.Valid || cfg.RemoteEtag.String != `"snap-1"` {
		t.Fatalf("remote_etag = %v, want \"snap-1\"", cfg.RemoteEtag)
	}
}

func TestSyncNowPullMergePush(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	// Remote snapshot with a task.
	remoteData := sampleSyncData()
	remoteData.Tasks = []Task{sampleTask(1, "task-remote-1", "2024-01-01T00:00:00Z")}
	remoteJSON, _ := json.Marshal(remoteData)

	fake := &fakeWebDAV{snapshot: string(remoteJSON), snapshotEtag: `"snap-0"`}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	configureSync(t, q, srv.URL, "pass")

	svc := newTestService(t, q, conn, t.TempDir())
	result, err := svc.SyncNow(ctx)
	if err != nil {
		t.Fatalf("sync now: %v", err)
	}
	if !result.Downloaded {
		t.Fatal("downloaded should be true")
	}
	if result.Stats.TasksMerged != 1 {
		t.Fatalf("tasks_merged = %d, want 1", result.Stats.TasksMerged)
	}

	// The remote task should now be in the local DB.
	tasks, err := q.ListAllTasksForSync(ctx)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Test Task" {
		t.Fatalf("local tasks = %+v", tasks)
	}
}

func TestSyncNowConflictRetryOnce(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	// The remote already has a snapshot (that is why the first upload 412s).
	remoteData := sampleSyncData()
	remoteJSON, _ := json.Marshal(remoteData)
	fake := &fakeWebDAV{snapshot: string(remoteJSON), snapshotEtag: `"snap-0"`, failUploadOnce: true}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	configureSync(t, q, srv.URL, "pass")

	svc := newTestService(t, q, conn, t.TempDir())
	result, err := svc.SyncNow(ctx)
	if err != nil {
		t.Fatalf("sync now: %v", err)
	}
	if !result.Uploaded {
		t.Fatal("uploaded should be true after retry")
	}
	if fake.uploadCount != 2 {
		t.Fatalf("upload count = %d, want 2 (one conflict + one retry)", fake.uploadCount)
	}
}

func TestSyncNowNotConfigured(t *testing.T) {
	q, conn := newTestStore(t)
	svc := newTestService(t, q, conn, t.TempDir())
	_, err := svc.SyncNow(context.Background())
	if err == nil || !strings.Contains(err.Error(), "未配置服务器地址") {
		t.Fatalf("err = %v, want not-configured error", err)
	}
}

func TestSyncNowAiSettingsSync(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	// Enable AI settings sync.
	if err := q.UpdateAiConfig(ctx, store.UpdateAiConfigParams{
		Enabled:         true,
		BaseUrl:         "https://api.deepseek.com",
		Model:           "deepseek-v4-flash",
		ApiKeyEncrypted: "",
		SyncEnabled:     true,
	}); err != nil {
		t.Fatalf("update ai config: %v", err)
	}

	fake := &fakeWebDAV{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	configureSync(t, q, srv.URL, "webdav-secret")

	dataDir := t.TempDir()
	svc := newTestService(t, q, conn, dataDir)
	if _, err := svc.SyncNow(ctx); err != nil {
		t.Fatalf("sync now: %v", err)
	}

	if fake.aiBlob == "" {
		t.Fatal("AI settings blob should have been uploaded")
	}
	// The blob must not leak the API key or model in plaintext.
	if strings.Contains(fake.aiBlob, "deepseek-v4-flash") {
		t.Fatal("AI blob leaks plaintext model")
	}

	// The DEK should be persisted locally.
	dek, err := loadDEK(dataDir)
	if err != nil || dek == nil {
		t.Fatalf("DEK should be persisted after first AI sync: %v", dek)
	}

	cfg, err := q.GetSyncConfig(ctx)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if !cfg.AiSettingsRemoteEtag.Valid || cfg.AiSettingsRemoteEtag.String != `"ai-1"` {
		t.Fatalf("ai_settings_remote_etag = %v, want \"ai-1\"", cfg.AiSettingsRemoteEtag)
	}
}

func TestSyncNowAiSettingsRemoteWins(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	// Remote AI blob encrypted with a known DEK. The remote updated_at is in
	// the future so it beats the local config's now() timestamp.
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)
	remotePayload := SyncAiPayload{
		Enabled:   true,
		BaseURL:   "https://remote.example.com",
		Model:     "remote-model",
		APIKey:    "sk-remote-key",
		UpdatedAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}
	blob, err := EncryptToBlob("webdav-secret", &remotePayload, dekArr)
	if err != nil {
		t.Fatalf("encrypt remote blob: %v", err)
	}

	fake := &fakeWebDAV{aiBlob: blob, aiEtag: `"ai-0"`}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	configureSync(t, q, srv.URL, "webdav-secret")

	if err := q.UpdateAiConfig(ctx, store.UpdateAiConfigParams{
		Enabled:         false,
		BaseUrl:         "https://local.example.com",
		Model:           "local-model",
		ApiKeyEncrypted: "",
		SyncEnabled:     true,
	}); err != nil {
		t.Fatalf("update ai config: %v", err)
	}

	svc := newTestService(t, q, conn, t.TempDir())
	if _, err := svc.SyncNow(ctx); err != nil {
		t.Fatalf("sync now: %v", err)
	}

	// Remote (newer) should win and be written to the local DB.
	cfg, err := q.GetAiConfig(ctx)
	if err != nil {
		t.Fatalf("get ai config: %v", err)
	}
	if cfg.BaseUrl != "https://remote.example.com" || cfg.Model != "remote-model" {
		t.Fatalf("ai config = %+v, want remote values", cfg)
	}
	if cfg.ApiKeyEncrypted == "" {
		t.Fatal("remote API key should be re-encrypted locally")
	}
}

func TestRecoveryKeyService(t *testing.T) {
	q, conn := newTestStore(t)
	dataDir := t.TempDir()
	svc := newTestService(t, q, conn, dataDir)

	key, err := svc.GetRecoveryKey(context.Background())
	if err != nil {
		t.Fatalf("get recovery key: %v", err)
	}
	if key != nil {
		t.Fatalf("recovery key should be nil before any sync, got %v", *key)
	}

	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)
	hexKey := RecoveryKeyHex(dekArr)
	if err := svc.SetRecoveryKey(context.Background(), hexKey); err != nil {
		t.Fatalf("set recovery key: %v", err)
	}
	got, err := svc.GetRecoveryKey(context.Background())
	if err != nil {
		t.Fatalf("get recovery key: %v", err)
	}
	if got == nil || *got != hexKey {
		t.Fatalf("recovery key = %v, want %q", got, hexKey)
	}

	if err := svc.SetRecoveryKey(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid recovery key should fail")
	}
}

func TestSyncNowTermPhasesMerge(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	phase := TermPhase{
		ID:                       1,
		SyncID:                   "phase-sync-1",
		TermLabel:                "2026S1",
		PhaseType:                "teaching",
		StartWeek:                1,
		EndWeek:                  16,
		AffectsCourses:           true,
		AffectsExamNotifications: true,
		PomodoroProfile:          "default",
		NotificationRulesJSON:    "{}",
		SortOrder:                0,
		CreatedAt:                "2024-01-01T00:00:00Z",
		UpdatedAt:                "2024-01-01T00:00:00Z",
	}
	data := sampleSyncData()
	data.TermPhases = []TermPhase{phase}

	stats, err := importAll(t, conn, q, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if stats.TermPhasesMerged != 1 {
		t.Fatalf("term_phases_merged = %d, want 1", stats.TermPhasesMerged)
	}

	// Newer remote phase should overwrite.
	newer := phase
	newer.UpdatedAt = "2024-02-01T00:00:00Z"
	newer.PomodoroProfile = "intense"
	data2 := sampleSyncData()
	data2.TermPhases = []TermPhase{newer}
	stats, err = importAll(t, conn, q, data2)
	if err != nil {
		t.Fatalf("merge import: %v", err)
	}
	if stats.TermPhasesMerged != 1 {
		t.Fatalf("term_phases_merged = %d, want 1", stats.TermPhasesMerged)
	}

	phases, err := q.ListAllTermPhases(ctx)
	if err != nil {
		t.Fatalf("list phases: %v", err)
	}
	if len(phases) != 1 || phases[0].PomodoroProfile != "intense" {
		t.Fatalf("phases = %+v", phases)
	}

	// Older remote should be rejected (conflict).
	older := phase
	older.UpdatedAt = "2023-12-01T00:00:00Z"
	older.PomodoroProfile = "relaxed"
	data3 := sampleSyncData()
	data3.TermPhases = []TermPhase{older}
	stats, err = importAll(t, conn, q, data3)
	if err != nil {
		t.Fatalf("older import: %v", err)
	}
	if stats.TermPhasesMerged != 0 || stats.Conflicts != 1 {
		t.Fatalf("stats = %+v, want merged=0 conflicts=1", stats)
	}
	phases, _ = q.ListAllTermPhases(ctx)
	if phases[0].PomodoroProfile != "intense" {
		t.Fatalf("older remote should not overwrite: %+v", phases[0])
	}
}

func TestSyncNowSessionsMergeWithTaskMapping(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	// Remote task + session referencing it.
	task := sampleTask(1, "task-sync-1", "2024-01-01T00:00:00Z")
	session := PomodoroSession{
		ID:          1,
		SyncID:      "session-sync-1",
		StartedAt:   "2024-01-01T08:00:00Z",
		EndedAt:     strPtr("2024-01-01T08:25:00Z"),
		SessionType: "work",
		TaskID:      int64Ptr(1),
	}
	data := sampleSyncData()
	data.Tasks = []Task{task}
	data.PomodoroSessions = []PomodoroSession{session}

	stats, err := importAll(t, conn, q, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if stats.TasksMerged != 1 || stats.SessionsMerged != 1 {
		t.Fatalf("stats = %+v", stats)
	}

	sessions, err := q.ListAllPomodoroSessionsForSync(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if !sessions[0].TaskID.Valid {
		t.Fatal("session task_id should be mapped to the local task")
	}
	tasks, _ := q.ListAllTasksForSync(ctx)
	if sessions[0].TaskID.Int64 != tasks[0].ID {
		t.Fatalf("session task_id = %d, want %d", sessions[0].TaskID.Int64, tasks[0].ID)
	}
}

func TestSyncNowSessionTombstoneWins(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	active := PomodoroSession{
		ID:          1,
		SyncID:      "session-sync-1",
		StartedAt:   "2024-01-01T08:00:00Z",
		SessionType: "work",
	}
	data := sampleSyncData()
	data.PomodoroSessions = []PomodoroSession{active}
	if _, err := importAll(t, conn, q, data); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	tombstone := active
	tombstone.DeletedAt = strPtr("2024-01-02T00:00:00Z")
	data2 := sampleSyncData()
	data2.PomodoroSessions = []PomodoroSession{tombstone}
	stats, err := importAll(t, conn, q, data2)
	if err != nil {
		t.Fatalf("tombstone import: %v", err)
	}
	if stats.SessionsMerged != 1 {
		t.Fatalf("sessions_merged = %d, want 1", stats.SessionsMerged)
	}

	sessions, err := q.ListAllPomodoroSessionsForSync(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || !sessions[0].DeletedAt.Valid {
		t.Fatalf("session tombstone should propagate: %+v", sessions)
	}
}

func TestSyncNowEmptyStats(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	fake := &fakeWebDAV{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	configureSync(t, q, srv.URL, "pass")

	svc := newTestService(t, q, conn, t.TempDir())
	result, err := svc.SyncNow(ctx)
	if err != nil {
		t.Fatalf("sync now: %v", err)
	}
	if result.Stats.TasksMerged != 0 || result.Stats.Conflicts != 0 {
		t.Fatalf("stats = %+v, want all zeros", result.Stats)
	}
}

func TestSyncNowPreservesRemoteEtagForConditionalUpload(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	remoteData := sampleSyncData()
	remoteJSON, _ := json.Marshal(remoteData)
	fake := &fakeWebDAV{snapshot: string(remoteJSON), snapshotEtag: `"snap-0"`}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	configureSync(t, q, srv.URL, "pass")

	svc := newTestService(t, q, conn, t.TempDir())
	if _, err := svc.SyncNow(ctx); err != nil {
		t.Fatalf("sync now: %v", err)
	}
	cfg, err := q.GetSyncConfig(ctx)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if !cfg.RemoteEtag.Valid || cfg.RemoteEtag.String != `"snap-1"` {
		t.Fatalf("remote_etag = %v, want \"snap-1\"", cfg.RemoteEtag)
	}
}

func TestSyncNowLastSyncAtFormat(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	fake := &fakeWebDAV{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	configureSync(t, q, srv.URL, "pass")

	svc := newTestService(t, q, conn, t.TempDir())
	if _, err := svc.SyncNow(ctx); err != nil {
		t.Fatalf("sync now: %v", err)
	}
	cfg, err := q.GetSyncConfig(ctx)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if !cfg.LastSyncAt.Valid {
		t.Fatal("last_sync_at should be valid")
	}
	// The stored value must be a UTC RFC3339 second-precision string.
	got := cfg.LastSyncAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	if got == "" {
		t.Fatal("last_sync_at should be a formatted timestamp")
	}
	_ = pgtype.Timestamptz{}
}
