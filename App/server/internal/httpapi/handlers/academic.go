package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"kairos/server/internal/domain/academic"
	"kairos/server/internal/domain/henu"
	"kairos/server/internal/domain/retention"
	"kairos/server/internal/httpapi/middleware"
	"kairos/server/internal/store"
)

const (
	maxUploadBytes  = 8 << 20
	maxRequestBytes = 9 << 20
)

// TxBeginner is satisfied by *pgxpool.Pool.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Academic serves the /api/academic/* endpoints for phase A.
type Academic struct {
	Q       *store.Queries
	Pool    TxBeginner
	Log     *slog.Logger
	DataDir string
	Now     func() time.Time
	// Henu carries the phase C direct-connection environment; nil means the
	// API was not configured for it and the endpoints report unavailable.
	Henu *HenuConnector
}

func (h *Academic) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Academic) owner(r *http.Request) string {
	return middleware.UsernameFrom(r.Context())
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]), hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]), hex.EncodeToString(b[10:16])), nil
}

// GetSemesterSettings handles GET /api/academic/semester-settings?semester=.
func (h *Academic) GetSemesterSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	semester := strings.TrimSpace(r.URL.Query().Get("semester"))
	if err := academic.ValidateSemester(semester); err != nil {
		writeAcademicError(w, err)
		return
	}
	owner := h.owner(r)

	row, err := h.Q.GetAcademicSettings(r.Context(), owner, semester)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, academic.DefaultSettings(semester))
		return
	}
	if err != nil {
		h.Log.Error("get academic settings", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	settings, err := settingsFromRow(row)
	if err != nil {
		h.Log.Error("decode academic settings", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// PutSemesterSettings handles PUT /api/academic/semester-settings.
func (h *Academic) PutSemesterSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in academic.SettingsInput
	if !decodeJSON(w, r, &in) {
		return
	}
	querySemester := strings.TrimSpace(r.URL.Query().Get("semester"))
	bodySemester := strings.TrimSpace(in.Semester)
	if querySemester != "" && bodySemester != "" && querySemester != bodySemester {
		writeErrorCode(w, http.StatusBadRequest, "query 与 body 中的 semester 不一致", "INVALID_ARGUMENT", nil)
		return
	}
	semester := querySemester
	if semester == "" {
		semester = bodySemester
	}
	if err := academic.ValidateSemester(semester); err != nil {
		writeAcademicError(w, err)
		return
	}
	normalized, err := academic.NormalizeSettings(semester, in)
	if err != nil {
		writeAcademicError(w, err)
		return
	}

	ctx := r.Context()
	owner := h.owner(r)

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.Log.Error("begin settings tx", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := h.Q.WithTx(tx)

	committed, err := qtx.CountCommittedBatches(ctx, owner, semester)
	if err != nil {
		h.Log.Error("count committed batches", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	current, err := qtx.GetAcademicSettings(ctx, owner, semester)
	currentExists := true
	if errors.Is(err, pgx.ErrNoRows) {
		currentExists = false
	} else if err != nil {
		h.Log.Error("read settings", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	currentRevision := int64(0)
	if currentExists {
		currentRevision = current.Revision
	}
	expected := int64(-1)
	if in.ExpectedRevision != nil {
		expected = *in.ExpectedRevision
	}
	if in.ExpectedRevision == nil || expected != currentRevision {
		writeErrorCode(w, http.StatusConflict, "配置已被修改，请刷新后重试", "REVISION_CONFLICT",
			map[string]any{"expected_revision": expected, "current_revision": currentRevision})
		return
	}

	normalized.Revision = currentRevision + 1
	row, anchorDate, err := rowFromSettings(owner, normalized)
	if err != nil {
		writeAcademicError(w, err)
		return
	}
	if currentExists {
		ok, err := qtx.UpdateAcademicSettings(ctx, owner, row, currentRevision, anchorDate)
		if err != nil {
			h.Log.Error("update settings", "error", err)
			writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
			return
		}
		if !ok {
			writeErrorCode(w, http.StatusConflict, "配置已被修改，请刷新后重试", "REVISION_CONFLICT", nil)
			return
		}
	} else if err := qtx.InsertAcademicSettings(ctx, owner, row, anchorDate); err != nil {
		h.Log.Error("insert settings", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	// With committed source courses, the same transaction must rebuild every
	// legacy projection from the new periods/anchor; a failure rolls it all back.
	if committed > 0 {
		sessions, err := qtx.ListAcademicState(ctx, owner, semester)
		if err != nil {
			h.Log.Error("list sessions for reprojection", "error", err)
			writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
			return
		}
		for _, s := range sessions {
			if !s.Active {
				continue
			}
			full, err := qtx.GetSourceSessionByID(ctx, s.SessionID)
			if err != nil {
				writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
				return
			}
			if _, err := academic.ProjectSession(ctx, qtx, owner, full); err != nil {
				var ve *academic.ValidationError
				if errors.As(err, &ve) {
					writeErrorCode(w, http.StatusConflict, ve.Message, ve.Code, ve.Details)
					return
				}
				h.Log.Error("reproject session", "error", err)
				writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
				return
			}
		}
	}

	if _, err := qtx.BumpAcademicVersions(ctx, owner, semester, 1, 0, 1); err != nil {
		h.Log.Error("bump academic versions", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.Log.Error("commit settings", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	writeJSON(w, http.StatusOK, normalized)
}

// CreatePreview handles POST /api/academic/henu/previews (multipart upload).
func (h *Academic) CreatePreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	ctx := r.Context()

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErrorCode(w, http.StatusRequestEntityTooLarge, "文件过大", "FILE_TOO_LARGE", nil)
			return
		}
		writeErrorCode(w, http.StatusBadRequest, "请求格式必须是 multipart/form-data", "INVALID_ARGUMENT", nil)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	semester := strings.TrimSpace(r.FormValue("semester"))
	if err := academic.ValidateSemester(semester); err != nil {
		writeAcademicError(w, err)
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) != 1 {
		writeErrorCode(w, http.StatusBadRequest, "必须且只能上传一个文件", "INVALID_ARGUMENT", nil)
		return
	}
	total := 0
	for _, list := range r.MultipartForm.File {
		total += len(list)
	}
	if total != 1 {
		writeErrorCode(w, http.StatusBadRequest, "必须且只能上传一个文件", "INVALID_ARGUMENT", nil)
		return
	}

	f, err := files[0].Open()
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "无法读取上传文件", "INVALID_ARGUMENT", nil)
		return
	}
	defer func() { _ = f.Close() }()

	raw, err := io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "无法读取上传文件", "INVALID_ARGUMENT", nil)
		return
	}
	if len(raw) > maxUploadBytes {
		writeErrorCode(w, http.StatusRequestEntityTooLarge, "文件超过 8 MiB", "FILE_TOO_LARGE", nil)
		return
	}
	if len(raw) == 0 {
		writeErrorCode(w, http.StatusUnprocessableEntity, "文件为空", "EMPTY_SOURCE", nil)
		return
	}

	parsed, err := henu.Parse(raw)
	if err != nil {
		switch {
		case errors.Is(err, henu.ErrUnsupportedFileType):
			writeErrorCode(w, http.StatusUnsupportedMediaType, "只支持 HTML 型 .xls/.html，不支持二进制 XLS/XLSX", "UNSUPPORTED_FILE_TYPE", nil)
		case errors.Is(err, henu.ErrLoginPage):
			writeErrorCode(w, http.StatusUnprocessableEntity, "该文件是登录页，不是课表", "UPSTREAM_AUTH_REQUIRED", nil)
		case errors.Is(err, henu.ErrEmptyTimetable):
			writeErrorCode(w, http.StatusUnprocessableEntity, "课表没有课程数据", "EMPTY_SOURCE", nil)
		default:
			writeErrorCode(w, http.StatusUnprocessableEntity, "无法识别该文档", "UNRECOGNIZED_DOCUMENT", nil)
		}
		return
	}

	preview, err := h.persistPreview(ctx, owner, semester, parsed, raw,
		academic.PreviewMeta{Acquisition: "FILE_IMPORT"}, nil)
	if err != nil {
		writeAcademicError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, preview)
}

// persistPreview is the shared "parsed result → snapshot → plan → stored
// preview" service behind file uploads and direct fetches. It runs inside one
// short transaction: versions are locked first, then the optional guard (used
// by the sync path to re-check connection generation and aggregate revision)
// runs, settings/state/plan follow and the row plus raw file land together.
func (h *Academic) persistPreview(ctx context.Context, owner, semester string, parsed *henu.Result, raw []byte, meta academic.PreviewMeta, guard func(*store.Queries) error) (*academic.Preview, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := h.Q.WithTx(tx)

	versions, err := qtx.LockAcademicVersions(ctx, owner, semester)
	if err != nil {
		return nil, err
	}
	if guard != nil {
		if err := guard(qtx); err != nil {
			return nil, err
		}
	}
	settings := academic.DefaultSettings(semester)
	row, err := qtx.GetAcademicSettings(ctx, owner, semester)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// default settings
	case err != nil:
		return nil, err
	default:
		settings, err = settingsFromRow(row)
		if err != nil {
			return nil, err
		}
	}
	state, err := loadExistingState(ctx, qtx, owner, semester)
	if err != nil {
		return nil, err
	}

	previewID, err := newUUID()
	if err != nil {
		return nil, err
	}
	now := h.now()
	preview, _, err := academic.BuildPreviewWithPlanMeta(parsed, settings, semester, academic.BaseVersions{
		SettingsRevision:  versions.SettingsRevision,
		TimetableRevision: versions.TimetableRevision,
		AggregateRevision: versions.AggregateRevision,
	}, previewID, now, state, nil, meta)
	if err != nil {
		return nil, err
	}

	// Store metadata and (when a data dir is configured) the raw upload.
	rawPath := ""
	if h.DataDir != "" {
		rawPath, err = h.storeRawPreview(previewID, raw)
		if err != nil {
			return nil, err
		}
	}

	sum := sha256.Sum256(raw)
	payload, err := json.Marshal(preview)
	if err != nil {
		if rawPath != "" {
			_ = os.Remove(rawPath)
		}
		return nil, err
	}
	summary, _ := json.Marshal(preview.Summary)
	issues, _ := json.Marshal(preview.Issues)

	row2 := store.AcademicPreviewRow{
		ID: previewID, Owner: owner, Semester: semester, Source: preview.Source,
		Acquisition: preview.Acquisition, SourceSemesterLabel: preview.SourceSemesterLabel,
		SourceObtainedAt: meta.SourceObtainedAt,
		ParserVersion:    preview.ParserVersion, Status: preview.Status, CanCommit: preview.CanCommit,
		Payload: payload, Summary: summary, Issues: issues,
		FileSHA256: hex.EncodeToString(sum[:]), FileBytes: int64(len(raw)),
		BaseSettingsRevision:  versions.SettingsRevision,
		BaseTimetableRevision: versions.TimetableRevision,
		BaseAggregateRevision: versions.AggregateRevision,
		ReceivedAt:            now, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour),
	}
	if rawPath != "" {
		row2.RawPath = &rawPath
	}
	if err := qtx.InsertAcademicPreview(ctx, row2); err != nil {
		if rawPath != "" {
			_ = os.Remove(rawPath)
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		if rawPath != "" {
			_ = os.Remove(rawPath)
		}
		return nil, err
	}
	// Expired previews are reclaimed by the background cleaner.
	return preview, nil
}

// GetPreview handles GET /api/academic/henu/previews/{preview_id}.
func (h *Academic) GetPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	id := strings.TrimSpace(chiURLParam(r, "preview_id"))
	if !uuidRe.MatchString(id) {
		writeErrorCode(w, http.StatusNotFound, "预览不存在", "NOT_FOUND", nil)
		return
	}
	row, err := h.Q.GetAcademicPreview(r.Context(), owner, id)
	if errors.Is(err, pgx.ErrNoRows) {
		// A known-but-expired preview answers 410 from its tombstone; anything
		// else (never existed, or another owner) stays 404.
		if tombstoned, terr := h.Q.GetTombstone(r.Context(), owner, id); terr == nil && tombstoned {
			writeErrorCode(w, http.StatusGone, "预览已过期", "PREVIEW_EXPIRED", nil)
			return
		}
		writeErrorCode(w, http.StatusNotFound, "预览不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		h.Log.Error("get preview", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	now := h.now()
	if row.CommittedAt == nil && !now.Before(row.ExpiresAt) {
		if err := h.reclaimExpired(r.Context(), row); err != nil {
			h.Log.Error("reclaim expired preview", "preview_id", row.ID, "error", err)
		}
		writeErrorCode(w, http.StatusGone, "预览已过期", "PREVIEW_EXPIRED", nil)
		return
	}

	// Committed preview details may be purged after the retention window; the
	// receipt stays in the batch table.
	if row.CommittedAt != nil && strings.TrimSpace(string(row.Payload)) == "{}" {
		writeJSON(w, http.StatusOK, map[string]any{
			"preview_id": row.ID, "status": "COMMITTED", "commit_id": row.CommitID,
		})
		return
	}

	var preview academic.Preview
	if err := json.Unmarshal(row.Payload, &preview); err != nil {
		h.Log.Error("decode preview payload", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	// Re-judge commitability against the current versions instead of trusting
	// the boolean stored at creation time. A failed check is an error, never a
	// silent "still fresh".
	versions, err := h.Q.GetAcademicVersions(r.Context(), owner, row.Semester)
	if err != nil {
		h.Log.Error("read versions for preview", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法校验预览版本", "SERVICE_UNAVAILABLE", nil)
		return
	}
	state := "FRESH"
	if versions.SettingsRevision != row.BaseSettingsRevision || versions.AggregateRevision != row.BaseAggregateRevision {
		state = "STALE"
		preview.CanCommit = false
	}
	preview.Validation = &academic.Validation{State: state, CurrentAggregateRevision: versions.AggregateRevision}
	if row.CommittedAt != nil {
		preview.Status = academic.StatusCommitted
		preview.CanCommit = false
		preview.CommitID = row.CommitID
	}
	writeJSON(w, http.StatusOK, preview)
}

// GetConnection lives in academic_c.go: phase C reports the real environment
// capabilities and the last verification state, still without any network
// activity on GET.

// loadExistingState loads the committed source state used for a real diff.
func (h *Academic) loadExistingState(ctx context.Context, owner, semester string) (academic.ExistingState, error) {
	return loadExistingState(ctx, h.Q, owner, semester)
}

func loadExistingState(ctx context.Context, q *store.Queries, owner, semester string) (academic.ExistingState, error) {
	rows, err := q.ListAcademicState(ctx, owner, semester)
	if err != nil {
		return academic.ExistingState{}, err
	}
	state := academic.ExistingState{}
	seen := map[string]bool{}
	for _, r := range rows {
		if !seen[r.CourseID] {
			seen[r.CourseID] = true
			state.Courses = append(state.Courses, academic.ExistingCourse{
				ID: r.CourseID, Name: r.CourseName, Code: r.CourseCode,
				ClassCode: r.ClassCode, Credits: r.Credits,
			})
		}
		values, err := academic.SourceValuesFromJSON(r.SourceValues)
		if err != nil {
			return academic.ExistingState{}, err
		}
		state.Sessions = append(state.Sessions, academic.ExistingSession{
			SessionID: r.SessionID, CourseID: r.CourseID, Active: r.Active,
			Values: values, LegacyCourseID: r.LegacyCourseID,
		})
	}
	return state, nil
}

// RebasePreview handles POST /api/academic/henu/previews/{preview_id}/rebase.
// It re-parses the stored raw upload against the current settings/state and
// returns a new preview; the source preview is left untouched and both keep the
// original 24h deadline.
func (h *Academic) RebasePreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	id := strings.TrimSpace(chiURLParam(r, "preview_id"))
	if !uuidRe.MatchString(id) {
		writeErrorCode(w, http.StatusNotFound, "预览不存在", "NOT_FOUND", nil)
		return
	}
	var body struct {
		ExpectedAggregateRevision int64                 `json:"expected_aggregate_revision"`
		ConfirmSemesterMapping    bool                  `json:"confirm_semester_mapping"`
		Resolutions               []academic.Resolution `json:"resolutions"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	ctx := r.Context()

	old, err := h.Q.GetAcademicPreview(ctx, owner, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "预览不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		h.Log.Error("load preview for rebase", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	now := h.now()
	if old.CommittedAt != nil {
		writeErrorCode(w, http.StatusConflict, "预览已提交，不能 rebase", "PREVIEW_NOT_READY", nil)
		return
	}
	if !now.Before(old.ExpiresAt) || old.RawPath == nil {
		writeErrorCode(w, http.StatusGone, "预览已过期", "PREVIEW_EXPIRED", nil)
		return
	}
	versions, err := h.Q.GetAcademicVersions(ctx, owner, old.Semester)
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法校验预览版本", "SERVICE_UNAVAILABLE", nil)
		return
	}
	if body.ExpectedAggregateRevision != versions.AggregateRevision {
		writeErrorCode(w, http.StatusConflict, "预览已过时，请刷新后重试", "REVISION_CONFLICT",
			map[string]any{"expected_revision": body.ExpectedAggregateRevision, "current_revision": versions.AggregateRevision})
		return
	}
	raw, err := os.ReadFile(*old.RawPath)
	if err != nil {
		writeErrorCode(w, http.StatusGone, "原始文件已回收，请重新上传", "PREVIEW_EXPIRED", nil)
		return
	}
	parsed, err := henu.Parse(raw)
	if err != nil {
		writeErrorCode(w, http.StatusUnprocessableEntity, "无法识别该文档", "UNRECOGNIZED_DOCUMENT", nil)
		return
	}
	settings, snap, err := h.settingsSnapshot(ctx, owner, old.Semester)
	if err != nil {
		h.Log.Error("settings snapshot", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	state, err := h.loadExistingState(ctx, owner, old.Semester)
	if err != nil {
		h.Log.Error("load state", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	newID, err := newUUID()
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	preview, _, err := academic.BuildPreviewWithPlanMeta(parsed, settings, old.Semester, academic.BaseVersions{
		SettingsRevision:  snap.SettingsRevision,
		TimetableRevision: snap.TimetableRevision,
		AggregateRevision: snap.AggregateRevision,
	}, newID, now, state, body.Resolutions, academic.PreviewMeta{
		Acquisition:      old.Acquisition,
		SourceObtainedAt: old.SourceObtainedAt,
	})
	if err != nil {
		writeAcademicError(w, err)
		return
	}
	if body.ConfirmSemesterMapping {
		filtered := preview.Issues[:0]
		for _, issue := range preview.Issues {
			if issue.Code == henu.CodeSemesterUnverified || issue.Code == henu.CodeSemesterMismatch {
				continue
			}
			filtered = append(filtered, issue)
		}
		preview.Issues = filtered
		academic.RecomputeStatus(preview)
	}
	// The family keeps the original deadline, not a fresh 24h.
	preview.ExpiresAt = academic.FormatTime(old.ExpiresAt)

	rawPath, err := h.storeRawPreview(newID, raw)
	if err != nil {
		h.Log.Error("store rebase raw", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	payload, _ := json.Marshal(preview)
	summary, _ := json.Marshal(preview.Summary)
	issuesJSON, _ := json.Marshal(preview.Issues)
	resolutionsJSON, _ := json.Marshal(body.Resolutions)
	sum := sha256.Sum256(raw)
	root := old.ID
	if old.RootPreviewID != nil && *old.RootPreviewID != "" {
		root = *old.RootPreviewID
	}
	row := store.AcademicPreviewRow{
		ID: newID, Owner: owner, Semester: old.Semester, Source: preview.Source,
		Acquisition: preview.Acquisition, SourceSemesterLabel: preview.SourceSemesterLabel,
		SourceObtainedAt: old.SourceObtainedAt,
		ParserVersion:    preview.ParserVersion, Status: preview.Status, CanCommit: preview.CanCommit,
		Payload: payload, Summary: summary, Issues: issuesJSON,
		FileSHA256: hex.EncodeToString(sum[:]), FileBytes: int64(len(raw)),
		BaseSettingsRevision:  snap.SettingsRevision,
		BaseTimetableRevision: snap.TimetableRevision,
		BaseAggregateRevision: snap.AggregateRevision,
		ReceivedAt:            now, CreatedAt: now, ExpiresAt: old.ExpiresAt,
		ParentPreviewID: &old.ID, RootPreviewID: &root, Resolutions: resolutionsJSON,
	}
	row.RawPath = &rawPath
	if err := h.Q.InsertAcademicPreview(ctx, row); err != nil {
		_ = os.Remove(rawPath)
		h.Log.Error("insert rebase preview", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	writeJSON(w, http.StatusCreated, preview)
}

// settingsSnapshot reads settings and counters inside one short transaction so
// the pair is consistent. Parsing and file work happen outside the lock.
func (h *Academic) settingsSnapshot(ctx context.Context, owner, semester string) (academic.Settings, store.AcademicVersions, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return academic.Settings{}, store.AcademicVersions{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := h.Q.WithTx(tx)

	versions, err := qtx.LockAcademicVersions(ctx, owner, semester)
	if err != nil {
		return academic.Settings{}, store.AcademicVersions{}, err
	}
	row, err := qtx.GetAcademicSettings(ctx, owner, semester)
	if errors.Is(err, pgx.ErrNoRows) {
		return academic.DefaultSettings(semester), versions, nil
	}
	if err != nil {
		return academic.Settings{}, store.AcademicVersions{}, err
	}
	settings, err := settingsFromRow(row)
	if err != nil {
		return academic.Settings{}, store.AcademicVersions{}, err
	}
	return settings, versions, nil
}

// reclaimExpired tombstones an expired preview and queues its file for removal.
func (h *Academic) reclaimExpired(ctx context.Context, row store.AcademicPreviewRow) error {
	cleaner := &retention.Cleaner{
		Q:    h.Q,
		Root: filepath.Join(h.DataDir, "academic", "previews"),
		Log:  h.Log,
		Now:  h.Now,
	}
	return cleaner.ReclaimNow(ctx, store.ExpiredPreview{
		ID: row.ID, Owner: row.Owner, Semester: row.Semester, RawPath: row.RawPath,
	})
}

func (h *Academic) storeRawPreview(previewID string, raw []byte) (string, error) {
	dir := filepath.Join(h.DataDir, "academic", "previews")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, previewID+".bin")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// writeAcademicError maps a validation error to the API error envelope.
func writeAcademicError(w http.ResponseWriter, err error) {
	var ve *academic.ValidationError
	if errors.As(err, &ve) {
		writeErrorCode(w, http.StatusBadRequest, ve.Message, ve.Code, ve.Details)
		return
	}
	writeErrorCode(w, http.StatusBadRequest, err.Error(), "INVALID_ARGUMENT", nil)
}

func settingsFromRow(row store.AcademicSettingsRow) (academic.Settings, error) {
	var periods []academic.Period
	if err := json.Unmarshal(row.Periods, &periods); err != nil {
		return academic.Settings{}, err
	}
	out := academic.Settings{
		SchemaVersion: 1,
		Semester:      row.Semester,
		Revision:      row.Revision,
		TimeZone:      row.TimeZone,
		SemesterLabel: row.SemesterLabel,
		WeekAnchor: academic.WeekAnchor{
			Source:    row.WeekAnchorSource,
			Confirmed: row.WeekAnchorConfirmed,
		},
		WeekStartDay: int(row.WeekStartDay),
		PeriodCount:  int(row.PeriodCount),
		Periods:      periods,
	}
	if row.WeekAnchorDate != nil {
		v := row.WeekAnchorDate.Format("2006-01-02")
		out.WeekAnchor.StartDate = &v
	}
	if row.TotalWeeks != nil {
		v := int(*row.TotalWeeks)
		out.TotalWeeks = &v
	}
	return out, nil
}

func rowFromSettings(owner string, s academic.Settings) (store.AcademicSettingsRow, *time.Time, error) {
	periods, err := json.Marshal(s.Periods)
	if err != nil {
		return store.AcademicSettingsRow{}, nil, err
	}
	var anchorDate *time.Time
	if s.WeekAnchor.StartDate != nil {
		t, err := time.Parse("2006-01-02", *s.WeekAnchor.StartDate)
		if err != nil {
			return store.AcademicSettingsRow{}, nil, err
		}
		anchorDate = &t
	}
	var totalWeeks *int32
	if s.TotalWeeks != nil {
		v := int32(*s.TotalWeeks)
		totalWeeks = &v
	}
	return store.AcademicSettingsRow{
		Semester:            s.Semester,
		Revision:            s.Revision,
		SemesterLabel:       s.SemesterLabel,
		TimeZone:            s.TimeZone,
		WeekAnchorSource:    s.WeekAnchor.Source,
		WeekAnchorConfirmed: s.WeekAnchor.Confirmed,
		WeekStartDay:        int32(s.WeekStartDay),
		TotalWeeks:          totalWeeks,
		PeriodCount:         int32(s.PeriodCount),
		Periods:             periods,
	}, anchorDate, nil
}
