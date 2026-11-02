package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"kairos/server/internal/domain/academic"
	"kairos/server/internal/domain/henu"
	"kairos/server/internal/schoolconn"
	"kairos/server/internal/store"
)

const (
	// loginSessionTTL bounds one interactive login attempt.
	loginSessionTTL = 10 * time.Minute
	// syncJobTimeout bounds the whole fetch task per the design.
	syncJobTimeout = 60 * time.Second
	// idempotencyTTL mirrors the B commit key retention.
	idempotencyTTL = 24 * time.Hour
)

// HenuConnector is the phase C direct-connection environment. All fields are
// fixed at startup; the zero value reports every capability as unavailable.
type HenuConnector struct {
	// BaseURL is the fixed school origin; tests point it at an httptest server.
	BaseURL string
	// KeyFile is the AES-256-GCM key file for session envelopes.
	KeyFile string
	// LocalInteractive enables opening a real browser window on this machine.
	LocalInteractive bool
	// Helper manages the Edge login helper process.
	Helper *schoolconn.LoginManager
	// NewClient builds the fixed-endpoint school client.
	NewClient func() (*schoolconn.Client, error)

	Log *slog.Logger
	Now func() time.Time

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func (c *HenuConnector) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *HenuConnector) logger() *slog.Logger {
	if c != nil && c.Log != nil {
		return c.Log
	}
	return slog.Default()
}

func (c *HenuConnector) registerCancel(key string, cancel context.CancelFunc) {
	c.mu.Lock()
	if c.cancels == nil {
		c.cancels = map[string]context.CancelFunc{}
	}
	c.cancels[key] = cancel
	c.mu.Unlock()
}

func (c *HenuConnector) cancelTask(key string) {
	c.mu.Lock()
	cancel, ok := c.cancels[key]
	c.mu.Unlock()
	if ok {
		cancel()
	}
}

func (c *HenuConnector) forgetCancel(key string) {
	c.mu.Lock()
	delete(c.cancels, key)
	c.mu.Unlock()
}

// sessionPayload is the decrypted session envelope content.
type sessionPayload struct {
	Cookies  []schoolconn.Cookie `json:"cookies"`
	UserCode string              `json:"user_code"`
}

// sessionEnvelope seals the payload, binding it to owner/school/generation.
func (c *HenuConnector) sessionEnvelope(owner string, generation int64, payload sessionPayload) ([]byte, error) {
	key, err := schoolconn.LoadSessionKey(c.KeyFile, true)
	if err != nil {
		return nil, err
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return schoolconn.Seal(key, plaintext, schoolconn.SessionAAD(owner, "henu", generation))
}

// openSession decrypts the stored envelope for the given generation.
func (c *HenuConnector) openSession(owner string, generation int64, ciphertext []byte) (*sessionPayload, error) {
	key, err := schoolconn.LoadSessionKey(c.KeyFile, false)
	if err != nil {
		return nil, schoolconn.ErrSessionUnreadable
	}
	plaintext, err := schoolconn.Open(key, ciphertext, schoolconn.SessionAAD(owner, "henu", generation))
	if err != nil {
		return nil, err
	}
	var payload sessionPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, schoolconn.ErrSessionUnreadable
	}
	return &payload, nil
}

// strictBody decodes exactly one JSON object into v with unknown fields
// rejected, mirroring the B commit contract.
func strictBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "请求格式不正确", "INVALID_ARGUMENT", nil)
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeErrorCode(w, http.StatusBadRequest, "请求必须是单个 JSON 对象", "INVALID_ARGUMENT", nil)
		return false
	}
	return true
}

// normalizedUUID validates and lowercases a UUID body field.
func normalizedUUID(value string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(value))
	if !uuidRe.MatchString(key) {
		return "", false
	}
	return key, true
}

// expectedTermLabel derives the school-side semester label from the verified
// term: xn=2026 / xqName=第一学期 becomes "2026-2027学年第一学期".
func expectedTermLabel(term *schoolconn.Term) (string, error) {
	year, err := strconv.Atoi(term.Year)
	if err != nil || year <= 0 {
		return "", fmt.Errorf("term year not numeric")
	}
	return fmt.Sprintf("%d-%d学年%s", year, year+1, term.Label), nil
}

// syncRequestSHA canonicalizes the request for idempotency: only the fields
// that define the work participate, never client noise.
func syncRequestSHA(semester string, expectedRevision int64) string {
	sum := sha256.Sum256([]byte("henu-sync-v1|" + semester + "|" + strconv.FormatInt(expectedRevision, 10)))
	return hex.EncodeToString(sum[:])
}

// loginJobView is the wire shape of a login task.
func loginJobView(row store.HenuLoginSessionRow) map[string]any {
	var errObj any
	if row.ErrorCode != nil {
		errObj = map[string]any{
			"code":        *row.ErrorCode,
			"message":     derefString(row.ErrorMessage),
			"occurred_at": formatOptTime(row.ErrorAt),
		}
	}
	return map[string]any{
		"login_id":    row.ID,
		"state":       row.State,
		"created_at":  academic.FormatTime(row.CreatedAt),
		"expires_at":  academic.FormatTime(row.ExpiresAt),
		"finished_at": formatOptTime(row.FinishedAt),
		"error":       errObj,
	}
}

// syncJobView is the wire shape of a fetch task.
func syncJobView(row store.HenuSyncJobRow) map[string]any {
	var errObj any
	if row.ErrorCode != nil {
		errObj = map[string]any{
			"code":        *row.ErrorCode,
			"message":     derefString(row.ErrorMessage),
			"occurred_at": formatOptTime(row.ErrorAt),
		}
	}
	return map[string]any{
		"job_id":      row.ID,
		"semester":    row.Semester,
		"state":       row.State,
		"created_at":  academic.FormatTime(row.CreatedAt),
		"started_at":  formatOptTime(row.StartedAt),
		"finished_at": formatOptTime(row.FinishedAt),
		"preview_id":  row.PreviewID,
		"error":       errObj,
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatOptTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return academic.FormatTime(*t)
}

// GetConnection handles GET /api/academic/henu/connection. It never touches
// the network: the state describes the last verification, not a liveness claim.
func (h *Academic) GetConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	ctx := r.Context()

	capabilities := map[string]bool{"file_import": true, "direct_fetch": false, "interactive_login": false}
	authMethods := []string{}
	if h.Henu != nil {
		if h.Henu.KeyFile != "" {
			capabilities["direct_fetch"] = true
		}
		if h.Henu.LocalInteractive && h.Henu.Helper.Available() {
			capabilities["interactive_login"] = true
			authMethods = []string{"LOCAL_EDGE_INTERACTIVE"}
		}
	}

	// Without a configured connector the endpoint keeps its phase A meaning:
	// UNAVAILABLE means "direct fetch does not exist in this deployment".
	// With a connector, a missing row is a real DISCONNECTED state.
	state := "DISCONNECTED"
	if h.Henu == nil {
		state = "UNAVAILABLE"
	}
	var lastVerified, lastChecked, lastErrorAt any
	var currentTerm any
	var lastError any
	row, err := h.Q.GetHenuConnection(ctx, owner)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// never connected
	case err != nil:
		h.Log.Error("read henu connection", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	default:
		state = row.State
		lastVerified = formatOptTime(row.VerifiedAt)
		if row.TermYear != nil && row.TermTerm != nil && row.TermLabel != nil {
			currentTerm = map[string]any{
				"upstream_year": *row.TermYear,
				"upstream_term": *row.TermTerm,
				"label":         *row.TermLabel,
			}
		}
		if row.LastErrorCode != nil {
			lastErrorAt = formatOptTime(row.LastErrorAt)
			lastError = map[string]any{
				"code":        *row.LastErrorCode,
				"message":     derefString(row.LastErrorMessage),
				"occurred_at": lastErrorAt,
			}
		}
	}
	if h.Q != nil {
		if at, err := h.Q.LatestHenuSyncFinishedAt(ctx, owner); err == nil {
			lastChecked = formatOptTime(at)
		}
	}
	var activeLogin, activeJob any
	if login, err := h.Q.GetActiveHenuLogin(ctx, owner); err == nil {
		activeLogin = login.ID
	}
	if job, err := h.Q.GetActiveHenuSyncJob(ctx, owner); err == nil {
		activeJob = job.ID
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"school":           "henu",
		"connection_state": state,
		"capabilities":     capabilities,
		"auth_methods":     authMethods,
		"active_login_id":  activeLogin,
		"active_job_id":    activeJob,
		"last_verified_at": lastVerified,
		"last_checked_at":  lastChecked,
		"current_term":     currentTerm,
		"last_error":       lastError,
	})
}

// CreateLoginSession handles POST /api/academic/henu/login-sessions.
func (h *Academic) CreateLoginSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	if h.Henu == nil || !h.Henu.LocalInteractive || h.Henu.Helper == nil || !h.Henu.Helper.Available() {
		writeErrorCode(w, http.StatusConflict, "本机交互登录在此环境不可用", "INTERACTIVE_LOGIN_UNAVAILABLE", nil)
		return
	}
	var body struct {
		Key *string `json:"idempotency_key"`
	}
	if !strictBody(w, r, &body) || body.Key == nil {
		writeErrorCode(w, http.StatusBadRequest, "idempotency_key 必填", "INVALID_ARGUMENT", nil)
		return
	}
	key, ok := normalizedUUID(*body.Key)
	if !ok {
		writeErrorCode(w, http.StatusBadRequest, "idempotency_key 必须是 UUID", "INVALID_ARGUMENT", nil)
		return
	}
	ctx := r.Context()

	// Same key replays the same task and never opens a second window.
	if existing, err := h.Q.GetHenuLoginSessionByKey(ctx, owner, key); err == nil {
		writeJSON(w, http.StatusAccepted, loginJobView(existing))
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("read login by key", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if existing, err := h.Q.GetActiveHenuLogin(ctx, owner); err == nil {
		writeErrorCode(w, http.StatusConflict, "已有进行中的登录任务", "LOGIN_IN_PROGRESS",
			map[string]any{"active_login_id": existing.ID})
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("read active login", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	loginID, err := newUUID()
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	now := h.Henu.now()
	expires := now.Add(loginSessionTTL)
	generation := int64(0)
	if row, err := h.Q.GetHenuConnection(ctx, owner); err == nil {
		generation = row.Generation
	}
	if err := h.Q.InsertHenuLoginSession(ctx, loginID, owner, generation, key, now, expires); err != nil {
		// A concurrent same-key creation wins; answer with its task.
		if existing, kerr := h.Q.GetHenuLoginSessionByKey(ctx, owner, key); kerr == nil {
			writeJSON(w, http.StatusAccepted, loginJobView(existing))
			return
		}
		h.Log.Error("insert login session", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	row, err := h.Q.GetHenuLoginSession(ctx, owner, loginID)
	if err != nil {
		h.Log.Error("read login session", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	go h.Henu.runLogin(h, owner, row)
	writeJSON(w, http.StatusAccepted, loginJobView(row))
}

// runLogin drives one interactive login: helper window → verify → save.
func (c *HenuConnector) runLogin(h *Academic, owner string, row store.HenuLoginSessionRow) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(context.Background()))
	c.registerCancel("login:"+row.ID, cancel)
	defer func() {
		c.forgetCancel("login:" + row.ID)
		cancel()
		c.Helper.CleanupProfile(row.ID)
	}()
	bg := ctx

	transition := func(from, to string, code, message *string) bool {
		ok, err := h.Q.TransitionHenuLogin(bg, owner, row.ID, from, to, code, message, nil, nil)
		if err != nil {
			c.logger().Error("transition login session", "error", err)
			return false
		}
		return ok
	}
	failTask := func(from, code, message string) {
		coded, msg := code, message
		transition(from, "FAILED", &coded, &msg)
	}

	outcome, runErr := c.Helper.Run(bg, row.ID, row.ExpiresAt)
	if errors.Is(runErr, context.DeadlineExceeded) {
		transition(row.State, "EXPIRED", nil, nil)
		return
	}
	if errors.Is(runErr, context.Canceled) {
		// Already marked by the canceller; nothing to write.
		return
	}
	if runErr != nil || outcome.Failed {
		message := outcome.Message
		if message == "" {
			message = "登录未完成"
		}
		failTask(row.State, "LOCAL_LOGIN_FAILED", message)
		return
	}

	if !transition(row.State, "VERIFYING", nil, nil) {
		// Cancelled or expired while the window was open; the profile cleanup
		// in defer is the only remaining duty.
		return
	}

	client, err := c.NewClient()
	if err != nil {
		failTask("VERIFYING", "LOCAL_LOGIN_FAILED", "无法创建学校客户端")
		return
	}
	if err := client.SetSession(outcome.Cookies); err != nil {
		failTask("VERIFYING", "LOCAL_LOGIN_FAILED", "会话数据无效")
		return
	}
	term, err := client.FetchTerm(bg)
	if errors.Is(err, schoolconn.ErrAuthRequired) {
		failTask("VERIFYING", "UPSTREAM_AUTH_REQUIRED", "学校要求重新登录，请确认登录的是教务账号")
		return
	}
	if errors.Is(err, schoolconn.ErrUpstreamUnavailable) {
		failTask("VERIFYING", "UPSTREAM_UNAVAILABLE", "学校暂时无法访问")
		return
	}
	if err != nil {
		failTask("VERIFYING", "UPSTREAM_SAFE_ERROR", "学校返回了无法确认的响应")
		return
	}

	// One transaction: compare subject fingerprint, advance the generation,
	// store the sealed session and close the task together.
	tx, err := h.Pool.Begin(bg)
	if err != nil {
		failTask("VERIFYING", "INTERNAL_ERROR", "数据库不可用")
		return
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(bg)) }()
	qtx := h.Q.WithTx(tx)

	connRow, err := qtx.GetHenuConnectionForUpdate(bg, owner)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		failTask("VERIFYING", "INTERNAL_ERROR", "数据库不可用")
		return
	}
	newGeneration := int64(1)
	if err == nil {
		newGeneration = connRow.Generation + 1
		if connRow.SessionCiphertext != nil {
			if old, err := c.openSession(owner, connRow.Generation, connRow.SessionCiphertext); err == nil {
				if old.UserCode != term.UserCode {
					failTask("VERIFYING", "UPSTREAM_ACCOUNT_CHANGED", "学校账号与上次授权不一致，请使用原账号重新登录")
					return
				}
			}
			// Old envelope unreadable (key loss): the old session is dead
			// either way; the new authorization replaces it.
		}
	}
	envelope, err := c.sessionEnvelope(owner, newGeneration, sessionPayload{Cookies: outcome.Cookies, UserCode: term.UserCode})
	if err != nil {
		failTask("VERIFYING", "LOCAL_LOGIN_FAILED", "会话加密失败")
		return
	}
	label, err := expectedTermLabel(term)
	if err != nil {
		failTask("VERIFYING", "UPSTREAM_SAFE_ERROR", "学校学期信息无法解析")
		return
	}
	if err := qtx.UpsertHenuConnected(bg, owner, newGeneration, envelope, term.Year, term.Term, label, c.now()); err != nil {
		failTask("VERIFYING", "INTERNAL_ERROR", "数据库不可用")
		return
	}
	started := c.now()
	if ok, err := qtx.TransitionHenuLogin(bg, owner, row.ID, "VERIFYING", "SUCCEEDED", nil, nil, &started, nil); err != nil || !ok {
		// Lost a cancel/expiry race; the connection save rolls back with the tx.
		_ = tx.Rollback(context.WithoutCancel(bg))
		return
	}
	if err := tx.Commit(bg); err != nil {
		c.logger().Error("commit login", "error", err)
	}
}

// GetLoginSession handles GET /api/academic/henu/login-sessions/{id}.
func (h *Academic) GetLoginSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	id := strings.TrimSpace(chiURLParam(r, "login_id"))
	if !uuidRe.MatchString(id) {
		writeErrorCode(w, http.StatusNotFound, "任务不存在", "NOT_FOUND", nil)
		return
	}
	row, err := h.Q.GetHenuLoginSession(r.Context(), owner, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "任务不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		h.Log.Error("read login session", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	writeJSON(w, http.StatusOK, loginJobView(row))
}

// DeleteLoginSession handles DELETE /api/academic/henu/login-sessions/{id}.
// It is idempotent and never rolls back an already-succeeded connection.
func (h *Academic) DeleteLoginSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	id := strings.TrimSpace(chiURLParam(r, "login_id"))
	if !uuidRe.MatchString(id) {
		writeErrorCode(w, http.StatusNotFound, "任务不存在", "NOT_FOUND", nil)
		return
	}
	row, err := h.Q.GetHenuLoginSession(r.Context(), owner, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "任务不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		h.Log.Error("read login session", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if row.State == "WAITING_FOR_USER" || row.State == "VERIFYING" {
		if _, err := h.Q.TransitionHenuLogin(r.Context(), owner, id, row.State, "CANCELLED", nil, nil, nil, nil); err != nil {
			h.Log.Error("cancel login session", "error", err)
			writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
			return
		}
		h.Henu.cancelTask("login:" + id)
	} else {
		// Terminal task: no side effects, but make sure no helper remnants
		// (e.g. after a crash) keep the profile directory alive.
		h.Henu.cancelTask("login:" + id)
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateSyncJob handles POST /api/academic/henu/sync-jobs.
func (h *Academic) CreateSyncJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	if h.Henu == nil || h.Henu.KeyFile == "" || h.Henu.NewClient == nil {
		writeErrorCode(w, http.StatusConflict, "直连抓取在此环境不可用", "DIRECT_FETCH_UNAVAILABLE", nil)
		return
	}
	var body struct {
		Semester *string `json:"semester"`
		Expected *int64  `json:"expected_aggregate_revision"`
		Key      *string `json:"idempotency_key"`
	}
	if !strictBody(w, r, &body) {
		return
	}
	if body.Semester == nil || academic.ValidateSemester(strings.TrimSpace(*body.Semester)) != nil {
		writeErrorCode(w, http.StatusBadRequest, "semester 必填且合法", "INVALID_ARGUMENT", nil)
		return
	}
	semester := strings.TrimSpace(*body.Semester)
	if body.Expected == nil || *body.Expected < 0 {
		writeErrorCode(w, http.StatusBadRequest, "expected_aggregate_revision 必填且非负", "INVALID_ARGUMENT", nil)
		return
	}
	if body.Key == nil {
		writeErrorCode(w, http.StatusBadRequest, "idempotency_key 必填", "INVALID_ARGUMENT", nil)
		return
	}
	key, ok := normalizedUUID(*body.Key)
	if !ok {
		writeErrorCode(w, http.StatusBadRequest, "idempotency_key 必须是 UUID", "INVALID_ARGUMENT", nil)
		return
	}
	expected := *body.Expected
	requestSHA := syncRequestSHA(semester, expected)
	ctx := r.Context()

	if existing, err := h.Q.GetHenuSyncJobByKey(ctx, owner, key); err == nil {
		if existing.RequestSHA256 == requestSHA {
			writeJSON(w, http.StatusAccepted, syncJobView(existing))
			return
		}
		writeErrorCode(w, http.StatusConflict, "该幂等键已被不同请求使用", "IDEMPOTENCY_CONFLICT", nil)
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("read sync job by key", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	connRow, err := h.Q.GetHenuConnection(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && connRow.SessionCiphertext == nil) {
		writeErrorCode(w, http.StatusConflict, "尚未连接河大", "CONNECTION_REQUIRED", nil)
		return
	}
	if err != nil {
		h.Log.Error("read connection", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if connRow.State == "AUTH_REQUIRED" {
		writeErrorCode(w, http.StatusConflict, "学校会话已失效，请重新授权", "UPSTREAM_AUTH_REQUIRED", nil)
		return
	}
	versions, err := h.Q.GetAcademicVersions(ctx, owner, semester)
	if err != nil {
		h.Log.Error("read versions", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法校验版本", "SERVICE_UNAVAILABLE", nil)
		return
	}
	if expected != versions.AggregateRevision {
		writeErrorCode(w, http.StatusConflict, "版本已变化，请刷新后重试", "REVISION_CONFLICT",
			map[string]any{"expected_revision": expected, "current_revision": versions.AggregateRevision})
		return
	}
	if existing, err := h.Q.GetActiveHenuSyncJob(ctx, owner); err == nil {
		writeErrorCode(w, http.StatusConflict, "已有进行中的检查更新", "SYNC_IN_PROGRESS",
			map[string]any{"active_job_id": existing.ID})
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("read active sync job", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	jobID, err := newUUID()
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if err := h.Q.InsertHenuSyncJob(ctx, jobID, owner, semester, connRow.Generation, expected, key, requestSHA); err != nil {
		if existing, kerr := h.Q.GetHenuSyncJobByKey(ctx, owner, key); kerr == nil {
			if existing.RequestSHA256 == requestSHA {
				writeJSON(w, http.StatusAccepted, syncJobView(existing))
				return
			}
			writeErrorCode(w, http.StatusConflict, "该幂等键已被不同请求使用", "IDEMPOTENCY_CONFLICT", nil)
			return
		}
		h.Log.Error("insert sync job", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	row, err := h.Q.GetHenuSyncJob(ctx, owner, jobID)
	if err != nil {
		h.Log.Error("read sync job", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	go h.Henu.runSyncJob(h, owner, row)
	writeJSON(w, http.StatusAccepted, syncJobView(row))
}

// GetSyncJob handles GET /api/academic/henu/sync-jobs/{id}.
func (h *Academic) GetSyncJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	id := strings.TrimSpace(chiURLParam(r, "job_id"))
	if !uuidRe.MatchString(id) {
		writeErrorCode(w, http.StatusNotFound, "任务不存在", "NOT_FOUND", nil)
		return
	}
	row, err := h.Q.GetHenuSyncJob(r.Context(), owner, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "任务不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		h.Log.Error("read sync job", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	writeJSON(w, http.StatusOK, syncJobView(row))
}

// runSyncJob executes one manual check-update task.
func (c *HenuConnector) runSyncJob(h *Academic, owner string, row store.HenuSyncJobRow) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(context.Background()))
	c.registerCancel("job:"+row.ID, cancel)
	defer func() {
		c.forgetCancel("job:" + row.ID)
		cancel()
	}()
	bg, timeoutCancel := context.WithTimeout(ctx, syncJobTimeout)
	defer timeoutCancel()

	transition := func(from, to string, code, message *string, startedAt, finishedAt, fetchedAt *time.Time, previewID *string) bool {
		ok, err := h.Q.TransitionHenuSyncJob(bg, owner, row.ID, from, to, code, message, startedAt, finishedAt, fetchedAt, previewID)
		if err != nil {
			c.logger().Error("transition sync job", "error", err)
		}
		return ok
	}
	// The guard needs the CURRENT state, not the one read at creation time:
	// once the job is RUNNING, every later transition starts from RUNNING.
	currentFrom := row.State
	failJob := func(code, message string, stateChange bool, newState string) {
		coded, msg := code, message
		now := c.now()
		if transition(currentFrom, "FAILED", &coded, &msg, nil, &now, nil, nil) && stateChange {
			if err := h.Q.SetHenuConnectionState(bg, owner, newState, code, message, now); err != nil {
				c.logger().Error("set connection state", "error", err)
			}
		}
	}

	now := c.now()
	if !transition(currentFrom, "RUNNING", nil, nil, &now, nil, nil, nil) {
		return // cancelled or interrupted before start
	}
	currentFrom = "RUNNING"

	connRow, err := h.Q.GetHenuConnection(bg, owner)
	if err != nil {
		failJob("LOCAL_ERROR", "读取连接失败", false, "")
		return
	}
	payload, err := c.openSession(owner, row.Generation, connRow.SessionCiphertext)
	if err != nil || payload == nil || len(payload.Cookies) == 0 {
		// Missing/corrupt key or ciphertext: the session must be re-authorized.
		failJob("UPSTREAM_AUTH_REQUIRED", "学校会话无法读取，请重新授权", true, "AUTH_REQUIRED")
		return
	}

	client, err := c.NewClient()
	if err != nil {
		failJob("LOCAL_ERROR", "无法创建学校客户端", false, "")
		return
	}
	if err := client.SetSession(payload.Cookies); err != nil {
		failJob("LOCAL_ERROR", "会话数据无效", false, "")
		return
	}

	term, err := client.FetchTerm(bg)
	if err != nil {
		c.classifyFetchError(h, owner, row, err, failJob)
		return
	}
	label, err := expectedTermLabel(term)
	if err != nil {
		failJob("UPSTREAM_SAFE_ERROR", "学校学期信息无法解析", false, "")
		return
	}
	settingsRow, err := h.Q.GetAcademicSettings(bg, owner, row.Semester)
	if errors.Is(err, pgx.ErrNoRows) {
		failJob("UPSTREAM_TERM_MISMATCH", "目标学期尚未配置学期标签，无法确认对应关系", false, "")
		return
	}
	if err != nil {
		failJob("LOCAL_ERROR", "读取学期设置失败", false, "")
		return
	}
	targetLabel := derefString(settingsRow.SemesterLabel)
	if targetLabel == "" || targetLabel != label {
		failJob("UPSTREAM_TERM_MISMATCH", "学校当前学期（"+label+"）与目标学期设置不一致", false, "")
		return
	}

	raw, err := client.FetchTimetable(bg, term)
	if err != nil {
		c.classifyFetchError(h, owner, row, err, failJob)
		return
	}
	fetchedAt := c.now()
	parsed, err := henu.Parse(raw)
	if err != nil {
		switch {
		case errors.Is(err, henu.ErrLoginPage):
			failJob("UPSTREAM_AUTH_REQUIRED", "学校返回登录页，请重新授权", true, "AUTH_REQUIRED")
		case errors.Is(err, henu.ErrEmptyTimetable):
			failJob("EMPTY_SOURCE", "学校返回了空课表", false, "")
		case errors.Is(err, henu.ErrUndecodable):
			failJob("INVALID_ENCODING", "学校响应编码异常", false, "")
		default:
			failJob("UNRECOGNIZED_DOCUMENT", "学校返回了无法识别的内容", false, "")
		}
		return
	}

	fetchGeneration := row.Generation
	expectedRevision := row.ExpectedRevision
	meta := academic.PreviewMeta{Acquisition: "DIRECT_FETCH", SourceObtainedAt: &fetchedAt}
	preview, err := h.persistPreview(bg, owner, row.Semester, parsed, raw, meta, func(qtx *store.Queries) error {
		versions, err := qtx.LockAcademicVersions(bg, owner, row.Semester)
		if err != nil {
			return err
		}
		if versions.AggregateRevision != expectedRevision {
			return &academicErr{status: http.StatusConflict, code: "REVISION_CONFLICT",
				message: "课程或设置已变化，请重新检查"}
		}
		connRow, err := qtx.GetHenuConnectionForUpdate(bg, owner)
		if err != nil {
			return &academicErr{status: http.StatusConflict, code: "REVISION_CONFLICT",
				message: "连接状态已变化，请重新检查"}
		}
		if connRow.Generation != fetchGeneration || connRow.SessionCiphertext == nil {
			return &academicErr{status: http.StatusConflict, code: "REVISION_CONFLICT",
				message: "连接已重新授权或断开，结果不再有效"}
		}
		return nil
	})
	if err != nil {
		var ae *academicErr
		if errors.As(err, &ae) && ae.code == "REVISION_CONFLICT" {
			failJob("REVISION_CONFLICT", "课程或设置已变化，请重新检查", false, "")
			return
		}
		failJob("INTERNAL_ERROR", "保存预览失败", false, "")
		return
	}

	finished := c.now()
	transition("RUNNING", "SUCCEEDED", nil, nil, nil, &finished, &fetchedAt, &preview.PreviewID)
}

// classifyFetchError maps school client errors to job failure codes and, where
// the design requires it, connection state changes.
func (c *HenuConnector) classifyFetchError(h *Academic, owner string, row store.HenuSyncJobRow, err error, failJob func(code, message string, stateChange bool, newState string)) {
	coded, msg := func() (string, string) {
		switch {
		case errors.Is(err, schoolconn.ErrAuthRequired):
			return "UPSTREAM_AUTH_REQUIRED", "学校要求重新登录"
		case errors.Is(err, schoolconn.ErrUpstreamUnavailable):
			return "UPSTREAM_UNAVAILABLE", "学校暂时无法访问，请稍后重试"
		case errors.Is(err, schoolconn.ErrResponseTooLarge):
			return "UPSTREAM_SAFE_ERROR", "学校响应超出大小限制"
		default:
			return "UPSTREAM_SAFE_ERROR", "学校返回了无法确认的响应"
		}
	}()
	stateChange := coded == "UPSTREAM_AUTH_REQUIRED"
	newState := ""
	if stateChange {
		newState = "AUTH_REQUIRED"
	} else if coded == "UPSTREAM_UNAVAILABLE" {
		newState = "TEMPORARY_FAILURE"
	}
	failJob(coded, msg, stateChange || coded == "UPSTREAM_UNAVAILABLE", newState)
}

// DeleteConnection handles DELETE /api/academic/henu/connection: one short
// transaction bumps the generation, clears credentials and cancels in-flight
// tasks; browser and directory cleanup happen outside it.
func (h *Academic) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	ctx := r.Context()
	now := h.now()

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.Log.Error("begin disconnect tx", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := h.Q.WithTx(tx)

	generation := int64(1)
	if row, err := qtx.GetHenuConnectionForUpdate(ctx, owner); err == nil {
		generation = row.Generation + 1
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("read connection for disconnect", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if err := qtx.DisconnectHenu(ctx, owner, generation); err != nil {
		h.Log.Error("disconnect", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if err := qtx.CancelOwnerHenuTasks(ctx, owner, now); err != nil {
		h.Log.Error("cancel owner tasks", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.Log.Error("commit disconnect", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	// In-flight goroutines observe their DB transitions failing and wind down;
	// cancelling them here speeds the browser close up. Without a connector
	// there is nothing in flight.
	if h.Henu != nil {
		if login, err := h.Q.GetActiveHenuLogin(ctx, owner); err == nil {
			h.Henu.cancelTask("login:" + login.ID)
		}
		if job, err := h.Q.GetActiveHenuSyncJob(ctx, owner); err == nil {
			h.Henu.cancelTask("job:" + job.ID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// RecoverInterrupted marks tasks orphaned by a restart as failed and cleans
// the login profile root. It must run before the first request is served.
func (c *HenuConnector) RecoverInterrupted(ctx context.Context, q *store.Queries) {
	if err := q.MarkNonterminalHenuLoginsFailed(ctx, "LOCAL_LOGIN_INTERRUPTED"); err != nil {
		c.logger().Error("recover login sessions", "error", err)
	}
	if err := q.MarkNonterminalHenuSyncJobsFailed(ctx, "JOB_INTERRUPTED"); err != nil {
		c.logger().Error("recover sync jobs", "error", err)
	}
	if c.Helper != nil && c.Helper.ProfileRoot != "" {
		entries, err := os.ReadDir(c.Helper.ProfileRoot)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					c.Helper.CleanupProfile(e.Name())
				}
			}
		}
	}
}
