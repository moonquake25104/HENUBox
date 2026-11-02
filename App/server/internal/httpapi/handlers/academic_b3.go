package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/domain/academic"
	"kairos/server/internal/domain/henu"
	"kairos/server/internal/store"
)

// commitBody is the fixed three-field commit contract. Pointers distinguish a
// missing field (or explicit null) from a real value.
type commitBody struct {
	Expected *int64    `json:"expected_aggregate_revision"`
	Key      *string   `json:"idempotency_key"`
	Ack      *[]string `json:"acknowledged_warning_codes"`
}

// CommitPreview handles POST /api/academic/henu/previews/{preview_id}/commit.
func (h *Academic) CommitPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	previewID := strings.TrimSpace(chiURLParam(r, "preview_id"))
	if !uuidRe.MatchString(previewID) {
		writeErrorCode(w, http.StatusNotFound, "预览不存在", "NOT_FOUND", nil)
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var body commitBody
	if err := dec.Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "提交请求格式不正确", "INVALID_ARGUMENT", nil)
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeErrorCode(w, http.StatusBadRequest, "提交请求必须是单个 JSON 对象", "INVALID_ARGUMENT", nil)
		return
	}
	if body.Expected == nil || *body.Expected < 0 {
		writeErrorCode(w, http.StatusBadRequest, "expected_aggregate_revision 必填且非负", "INVALID_ARGUMENT", nil)
		return
	}
	if body.Key == nil {
		writeErrorCode(w, http.StatusBadRequest, "idempotency_key 必填", "INVALID_ARGUMENT", nil)
		return
	}
	idempotencyKey := strings.ToLower(strings.TrimSpace(*body.Key))
	if !uuidRe.MatchString(idempotencyKey) {
		writeErrorCode(w, http.StatusBadRequest, "idempotency_key 必须是 UUID", "INVALID_ARGUMENT", nil)
		return
	}
	if body.Ack == nil {
		writeErrorCode(w, http.StatusBadRequest, "acknowledged_warning_codes 必填（无警告传 []）", "INVALID_ARGUMENT", nil)
		return
	}
	acks := normalizedAckCodes(*body.Ack)
	expectedRevision := *body.Expected
	requestSHA, err := hashCommitRequest(previewID, expectedRevision, acks)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "请求无法规范化", "INVALID_ARGUMENT", nil)
		return
	}

	// Idempotent replay: same key + same request returns the original receipt.
	if _, sha, receipt, err := h.Q.GetCommitRequestByKey(r.Context(), owner, idempotencyKey); err == nil {
		if sha != requestSHA {
			writeErrorCode(w, http.StatusConflict, "该幂等键已用于另一个请求", "IDEMPOTENCY_CONFLICT", nil)
			return
		}
		writeRawJSON(w, http.StatusOK, receipt)
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("load idempotency record", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	row, err := h.Q.GetAcademicPreview(r.Context(), owner, previewID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "预览不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		h.Log.Error("load preview for commit", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	now := h.now()
	if row.CommittedAt != nil {
		if row.CommitID != nil {
			if receipt, _, err := h.Q.GetCommitByID(r.Context(), owner, *row.CommitID); err == nil {
				out, aerr := h.registerCommitAlias(r.Context(), owner, idempotencyKey, requestSHA, *row.CommitID, receipt)
				if aerr != nil {
					h.writeAliasError(w, aerr)
					return
				}
				writeRawJSON(w, http.StatusOK, out)
				return
			}
		}
		writeErrorCode(w, http.StatusConflict, "预览已提交", "PREVIEW_NOT_READY", nil)
		return
	}
	if !now.Before(row.ExpiresAt) {
		writeErrorCode(w, http.StatusGone, "预览已过期", "PREVIEW_EXPIRED", nil)
		return
	}

	versions, err := h.Q.GetAcademicVersions(r.Context(), owner, row.Semester)
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法校验版本", "SERVICE_UNAVAILABLE", nil)
		return
	}
	if expectedRevision != versions.AggregateRevision ||
		row.BaseAggregateRevision != versions.AggregateRevision ||
		row.BaseSettingsRevision != versions.SettingsRevision {
		writeErrorCode(w, http.StatusConflict, "预览已过时，请刷新后重试", "REVISION_CONFLICT",
			map[string]any{"expected_revision": expectedRevision, "current_revision": versions.AggregateRevision})
		return
	}

	var preview academic.Preview
	if err := json.Unmarshal(row.Payload, &preview); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	// A rebased preview carries the decisions that produced it; commit only
	// applies those persisted decisions and never opens a second identity flow.
	var storedResolutions []academic.Resolution
	if len(row.Resolutions) > 0 {
		_ = json.Unmarshal(row.Resolutions, &storedResolutions)
	}
	resolutions := append([]academic.Resolution{}, storedResolutions...)
	state, err := h.loadExistingState(r.Context(), owner, row.Semester)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	// Recompute the diff from scratch with the commit-time resolutions; the
	// stored preview diff is only a snapshot.
	preview.Diff = academic.Diff{
		Added: []string{}, Changed: []academic.ChangedSession{}, Unchanged: []string{},
		Missing: []academic.MissingSession{}, Ambiguous: []academic.AmbiguousSession{},
	}
	plan, err := academic.ComputePlan(&preview, state, resolutions)
	if err != nil {
		writeAcademicError(w, err)
		return
	}
	// Identity issues were resolved by the commit-time decisions; keep only
	// content/configuration issues when recomputing the status.
	filtered := preview.Issues[:0]
	for _, issue := range preview.Issues {
		if issue.Code == "IDENTITY_RESOLUTION_REQUIRED" || issue.Code == "MISSING_SESSION_DECISION" {
			continue
		}
		filtered = append(filtered, issue)
	}
	preview.Issues = filtered
	academic.RecomputeStatus(&preview)
	if plan.Unresolved {
		writeErrorCode(w, http.StatusConflict, "仍有身份歧义或未决定的缺席安排", "IDENTITY_RESOLUTION_REQUIRED", nil)
		return
	}
	if preview.Status != academic.StatusReady {
		writeErrorCode(w, http.StatusConflict, "预览尚未达到可提交状态", "PREVIEW_NOT_READY",
			map[string]any{"status": preview.Status})
		return
	}
	if missing, extra := validateWarningAcks(preview.Issues, acks); len(missing) > 0 || len(extra) > 0 {
		writeErrorCode(w, http.StatusBadRequest, "警告确认码不完整或非法", "WARNING_ACK_REQUIRED",
			map[string]any{"missing": missing, "invalid": extra})
		return
	}

	settings, snap, err := h.settingsSnapshot(r.Context(), owner, row.Semester)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	_ = snap

	commitID, _ := academic.NewUUID()
	batchID := commitID
	receipt := academic.CommitResult{
		CommitID: commitID, PreviewID: previewID, Status: "COMMITTED",
		CommittedAt: academic.FormatTime(now), WarningsAcknowledged: acks,
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(r.Context())) }()
	qtx := h.Q.WithTx(tx)

	if err := academic.LockPartitions(r.Context(), qtx, owner, []string{row.Semester}); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	locked, err := qtx.GetAcademicPreviewForUpdate(r.Context(), owner, previewID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "预览不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	if locked.CommittedAt != nil {
		if receipt2, _, err := qtx.GetCommitByID(r.Context(), owner, *locked.CommitID); err == nil {
			_ = tx.Rollback(r.Context())
			out, aerr := h.registerCommitAlias(r.Context(), owner, idempotencyKey, requestSHA, *locked.CommitID, receipt2)
			if aerr != nil {
				h.writeAliasError(w, aerr)
				return
			}
			writeRawJSON(w, http.StatusOK, out)
			return
		}
		writeErrorCode(w, http.StatusConflict, "预览已提交", "PREVIEW_NOT_READY", nil)
		return
	}
	// Re-check every gate under the partition lock: a different preview with the
	// same base revision must not slip through after the first commit bumped it.
	lockedVersions, err := qtx.GetAcademicVersions(r.Context(), owner, row.Semester)
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法校验版本", "SERVICE_UNAVAILABLE", nil)
		return
	}
	if expectedRevision != lockedVersions.AggregateRevision ||
		locked.BaseSettingsRevision != lockedVersions.SettingsRevision ||
		locked.BaseAggregateRevision != lockedVersions.AggregateRevision {
		writeErrorCode(w, http.StatusConflict, "预览已过时，请刷新后重试", "REVISION_CONFLICT",
			map[string]any{"expected_revision": expectedRevision, "current_revision": lockedVersions.AggregateRevision})
		return
	}
	if !h.now().Before(locked.ExpiresAt) {
		writeErrorCode(w, http.StatusGone, "预览已过期", "PREVIEW_EXPIRED", nil)
		return
	}

	// The batch row must exist before projections reference it (FK), so insert
	// a placeholder and fill the result at the end of the same transaction.
	// The acquisition provenance comes from the committed preview row.
	if err := qtx.InsertBatch(r.Context(), batchID, owner, row.Semester, &previewID, locked.RootPreviewID,
		preview.ParserVersion, versions.SettingsRevision, versions.AggregateRevision,
		[]byte("{}"), []byte("{}"), []byte("[]"), false, &idempotencyKey,
		locked.Acquisition, locked.SourceObtainedAt); err != nil {
		h.Log.Error("insert batch placeholder", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "提交失败，已回滚", "INTERNAL_ERROR", nil)
		return
	}

	retired := 0
	for _, m := range preview.Diff.Missing {
		if m.Decision != nil && *m.Decision == academic.DecisionRetire {
			retired++
		}
	}
	noChanges := len(preview.Diff.Added) == 0 && len(preview.Diff.Changed) == 0 && retired == 0
	counts := academic.CommitCounts{}
	if noChanges {
		counts.Unchanged = len(preview.Diff.Unchanged)
		for _, m := range preview.Diff.Missing {
			if m.Decision != nil && *m.Decision == academic.DecisionRetain {
				counts.Retained++
			}
		}
	} else {
		counts, err = academic.CommitImport(r.Context(), qtx, academic.CommitInput{
			Owner: owner, Semester: row.Semester, CommitID: commitID, BatchID: batchID,
			Preview: &preview, Plan: plan, Settings: settings, Now: now,
		})
		if err != nil {
			h.Log.Error("commit import", "error", err)
			writeErrorCode(w, http.StatusInternalServerError, "提交失败，已回滚", "INTERNAL_ERROR", nil)
			return
		}
	}
	receipt.Counts = counts
	receipt.NoChanges = noChanges
	receipt.Semester = row.Semester
	receipt.Revision = lockedVersions.TimetableRevision
	receipt.AggregateRevision = lockedVersions.AggregateRevision
	if !noChanges {
		receipt.Revision++
		receipt.AggregateRevision++
	}
	receipt.ReminderRecomputeState = "NOT_REQUIRED"
	legacyIDs := []int64{}
	if rows, err := qtx.ListAcademicState(r.Context(), owner, row.Semester); err == nil {
		seen := map[int64]bool{}
		for _, sr := range rows {
			if sr.LegacyCourseID != nil && !seen[*sr.LegacyCourseID] {
				seen[*sr.LegacyCourseID] = true
				legacyIDs = append(legacyIDs, *sr.LegacyCourseID)
			}
		}
	}
	receipt.LegacyCourseIDs = legacyIDs

	receiptJSON, _ := json.Marshal(receipt)
	countsJSON, _ := json.Marshal(counts)
	decisionsJSON, _ := json.Marshal(resolutions)
	if err := qtx.UpdateBatchResult(r.Context(), batchID, countsJSON, receiptJSON, decisionsJSON, receipt.NoChanges); err != nil {
		h.Log.Error("update batch", "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "提交失败，已回滚", "INTERNAL_ERROR", nil)
		return
	}
	if !noChanges {
		if _, err := qtx.BumpAcademicVersions(r.Context(), owner, row.Semester, 0, 1, 1); err != nil {
			writeErrorCode(w, http.StatusInternalServerError, "提交失败，已回滚", "INTERNAL_ERROR", nil)
			return
		}
	}
	if locked.RawPath != nil {
		if err := qtx.EnqueuePreviewFile(r.Context(), previewID, owner, *locked.RawPath); err != nil {
			writeErrorCode(w, http.StatusInternalServerError, "提交失败，已回滚", "INTERNAL_ERROR", nil)
			return
		}
	}
	if err := qtx.InsertCommitRequest(r.Context(), commitID, owner, idempotencyKey, requestSHA,
		commitID, receiptJSON, now.Add(30*24*time.Hour)); err != nil {
		_ = tx.Rollback(r.Context())
		if _, sha, existing, gerr := h.Q.GetCommitRequestByKey(r.Context(), owner, idempotencyKey); gerr == nil {
			if sha == requestSHA {
				writeRawJSON(w, http.StatusOK, existing)
				return
			}
			writeErrorCode(w, http.StatusConflict, "该幂等键已用于另一个请求", "IDEMPOTENCY_CONFLICT", nil)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, "提交失败，已回滚", "INTERNAL_ERROR", nil)
		return
	}
	if err := qtx.MarkPreviewCommitted(r.Context(), owner, previewID, commitID, now); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "提交失败，已回滚", "INTERNAL_ERROR", nil)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "提交失败，已回滚", "INTERNAL_ERROR", nil)
		return
	}
	// The raw upload is no longer needed once committed: drop it now; if the
	// delete fails the queued task retries it later.
	if locked.RawPath != nil {
		if err := os.Remove(*locked.RawPath); err == nil || os.IsNotExist(err) {
			_ = h.Q.CompletePreviewFile(r.Context(), previewID)
			_ = h.Q.ClearPreviewRawPath(r.Context(), owner, previewID)
		} else {
			h.Log.Warn("remove committed raw upload queued for retry", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, receipt)
}

// GetCommit handles GET /api/academic/commits/{commit_id}.
func (h *Academic) GetCommit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	id := strings.TrimSpace(chiURLParam(r, "commit_id"))
	if !uuidRe.MatchString(id) {
		writeErrorCode(w, http.StatusNotFound, "提交记录不存在", "NOT_FOUND", nil)
		return
	}
	receipt, _, err := h.Q.GetCommitByID(r.Context(), owner, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "提交记录不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	writeRawJSON(w, http.StatusOK, receipt)
}

type overrideRequest struct {
	ExpectedAggregateRevision int64          `json:"expected_aggregate_revision"`
	Set                       map[string]any `json:"set"`
	ResetFields               []string       `json:"reset_fields"`
}

// PatchSessionOverrides handles PATCH /api/academic/sessions/{session_id}/overrides.
// Ownership, version check, override merge, projection and version bump all run
// inside one locked transaction.
func (h *Academic) PatchSessionOverrides(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	sessionID := strings.TrimSpace(chiURLParam(r, "session_id"))
	if !uuidRe.MatchString(sessionID) {
		writeErrorCode(w, http.StatusNotFound, "安排不存在", "NOT_FOUND", nil)
		return
	}
	var body overrideRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	ctx := r.Context()
	// Locate the semester first; the authoritative re-read happens under lock.
	located, err := h.Q.GetSourceSessionForOwner(ctx, owner, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "安排不存在", "NOT_FOUND", nil)
		return
	}
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}

	var updated store.SourceSessionRow
	finalVersions, err := h.AcademicTxVersions(ctx, owner, located.Semester, func(q *store.Queries) (bool, error) {
		current, err := q.GetSourceSessionForOwner(ctx, owner, sessionID)
		if err != nil {
			return false, err
		}
		versions, err := q.GetAcademicVersions(ctx, owner, current.Semester)
		if err != nil {
			return false, err
		}
		if body.ExpectedAggregateRevision != versions.AggregateRevision {
			return false, &academicErr{status: http.StatusConflict, code: "REVISION_CONFLICT",
				message: "数据已变化，请刷新后重试",
				details: map[string]any{"expected_revision": body.ExpectedAggregateRevision, "current_revision": versions.AggregateRevision}}
		}
		old := academic.OverridesFromRow(current.SetFields, current.Name, current.Teacher, current.Location, current.Color, current.Hidden)
		merged, err := applyOverridePatch(old, body.Set, body.ResetFields)
		if err != nil {
			return false, err
		}
		if sameOverrides(old, merged) {
			updated = current
			return false, nil
		}
		if err := q.UpsertSessionOverride(ctx, sessionID, merged.SetFields, merged.Name, merged.Teacher, merged.Location, merged.Color, merged.Hidden); err != nil {
			return false, err
		}
		after, err := q.GetSourceSessionForOwner(ctx, owner, sessionID)
		if err != nil {
			return false, err
		}
		if _, err := academic.ProjectSession(ctx, q, owner, after); err != nil {
			return false, err
		}
		updated = after
		return true, nil
	})
	if err != nil {
		h.writeMutationError(w, err)
		return
	}
	source, err := academic.SourceValuesFromJSON(updated.SourceValues)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
		return
	}
	effective := academic.Effective(source, academic.OverridesFromRow(updated.SetFields, updated.Name, updated.Teacher, updated.Location, updated.Color, updated.Hidden))
	var legacyIDs []int64
	if updated.LegacyCourseID != nil {
		legacyIDs = append(legacyIDs, *updated.LegacyCourseID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":         sessionID,
		"source_values":      source,
		"effective_values":   effective,
		"overridden_fields":  updated.SetFields,
		"legacy_course_ids":  legacyIDs,
		"revision":           finalVersions.TimetableRevision,
		"aggregate_revision": finalVersions.AggregateRevision,
	})
}

// Timetable handles GET /api/academic/timetable?semester= with the contracted
// v1 shape, read from one repeatable-read snapshot.
func (h *Academic) Timetable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := h.owner(r)
	semester := strings.TrimSpace(r.URL.Query().Get("semester"))
	if err := academic.ValidateSemester(semester); err != nil {
		writeAcademicError(w, err)
		return
	}
	ctx := r.Context()
	// Make sure the counter row exists before opening a read-only snapshot.
	if _, err := h.Q.GetAcademicVersions(ctx, owner, semester); err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ"); err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	if _, err := tx.Exec(ctx, "SET TRANSACTION READ ONLY"); err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	qtx := h.Q.WithTx(tx)

	versions, err := qtx.SelectAcademicVersions(ctx, owner, semester)
	if err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	settingsRow, err := qtx.GetAcademicSettings(ctx, owner, semester)
	settings := academic.DefaultSettings(semester)
	if err == nil {
		settings, err = settingsFromRow(settingsRow)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
			return
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	rows, err := qtx.ListAcademicState(ctx, owner, semester)
	if err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	linked, err := qtx.ListLegacyLinkedCourseIDs(ctx)
	if err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	manual, err := qtx.ListCourses(ctx, pgtype.Text{})
	if err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	lastImport, err := qtx.LatestCommitAt(ctx, owner, semester)
	if err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	pending, err := qtx.LatestPendingPreviewID(ctx, owner, semester, h.now())
	if err != nil {
		h.Log.Error("timetable read failed", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}

	type sessionOut struct {
		SessionID        string          `json:"session_id"`
		SourceEntryID    string          `json:"source_entry_id"`
		AcademicCourseID string          `json:"academic_course_id"`
		Active           bool            `json:"active"`
		SourceValues     academic.Values `json:"source_values"`
		EffectiveValues  academic.Values `json:"effective_values"`
		OverriddenFields []string        `json:"overridden_fields"`
		LegacyCourseIDs  []int64         `json:"legacy_course_ids"`
	}
	type courseOut struct {
		ID                        string       `json:"id"`
		Semester                  string       `json:"semester"`
		Source                    string       `json:"source"`
		UpstreamCourseCode        *string      `json:"upstream_course_code"`
		UpstreamTeachingClassCode *string      `json:"upstream_teaching_class_code"`
		Name                      string       `json:"name"`
		Credits                   *string      `json:"credits"`
		Sessions                  []sessionOut `json:"sessions"`
	}

	order := []string{}
	byCourse := map[string]*courseOut{}
	for _, row := range rows {
		values, err := academic.SourceValuesFromJSON(row.SourceValues)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
			return
		}
		overrideRow, err := qtx.GetSourceSessionForOwner(ctx, owner, row.SessionID)
		if err != nil {
			h.Log.Error("timetable read failed", "error", err)
			writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
			return
		}
		effective := academic.Effective(values, academic.OverridesFromRow(overrideRow.SetFields, overrideRow.Name, overrideRow.Teacher, overrideRow.Location, overrideRow.Color, overrideRow.Hidden))
		if !row.Active {
			effective.Hidden = true
		}
		course, ok := byCourse[row.CourseID]
		if !ok {
			course = &courseOut{
				ID: row.CourseID, Semester: semester, Source: "henu",
				UpstreamCourseCode: row.CourseCode, UpstreamTeachingClassCode: row.ClassCode,
				Name: row.CourseName, Credits: row.Credits, Sessions: []sessionOut{},
			}
			byCourse[row.CourseID] = course
			order = append(order, row.CourseID)
		}
		ids := []int64{}
		if row.LegacyCourseID != nil {
			ids = append(ids, *row.LegacyCourseID)
		}
		course.Sessions = append(course.Sessions, sessionOut{
			SessionID: row.SessionID, SourceEntryID: row.SourceEntryID, AcademicCourseID: row.CourseID,
			Active: row.Active, SourceValues: values, EffectiveValues: effective,
			OverriddenFields: append([]string{}, overrideRow.SetFields...), LegacyCourseIDs: ids,
		})
	}
	courses := []courseOut{}
	for _, id := range order {
		courses = append(courses, *byCourse[id])
	}
	legacy := []store.Course{}
	for _, c := range manual {
		if !linked[c.ID] {
			legacy = append(legacy, c)
		}
	}
	state := "NEVER_IMPORTED"
	var lastImportText *string
	if lastImport != nil {
		state = "READY"
		v := academic.FormatTime(*lastImport)
		lastImportText = &v
	}
	// Acquisition provenance comes from the newest committed batch: a file
	// import reports FILE_IMPORT with a null fetch time, a direct fetch
	// reports DIRECT_FETCH with the real school response time. Uncommitted
	// previews never change it; an empty semester has no acquisition at all.
	acquisition := "NONE"
	var dataFetchedAt any
	if batchAcq, batchObtained, err := qtx.LatestCommittedBatchAcquisition(ctx, owner, semester); err == nil {
		acquisition = batchAcq
		if batchObtained != nil {
			v := academic.FormatTime(*batchObtained)
			dataFetchedAt = v
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("read latest batch acquisition", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	// last_checked_at / last_error describe the newest finished fetch check of
	// this semester, success or failure; they never claim the school session
	// is alive (that is /connection's job).
	var lastCheckedAt any
	var syncLastError any
	if job, err := qtx.LatestFinishedHenuSyncJob(ctx, owner, semester); err == nil {
		if job.FinishedAt != nil {
			lastCheckedAt = academic.FormatTime(*job.FinishedAt)
		}
		if job.ErrorCode != nil {
			syncLastError = map[string]any{
				"code":        *job.ErrorCode,
				"message":     derefString(job.ErrorMessage),
				"occurred_at": formatOptTime(job.ErrorAt),
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("read latest sync job", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "暂时无法读取课表", "SERVICE_UNAVAILABLE", nil)
		return
	}
	resp := map[string]any{
		"schema_version":     1,
		"semester":           semester,
		"time_zone":          settings.TimeZone,
		"revision":           versions.TimetableRevision,
		"aggregate_revision": versions.AggregateRevision,
		"settings":           settings,
		"courses":            courses,
		"legacy_courses":     legacy,
		"pending_preview_id": pending,
		"acquisition":        acquisition,
		"source_sync": map[string]any{
			"source": "henu", "state": state, "data_fetched_at": dataFetchedAt,
			"last_successful_import_at": lastImportText, "last_checked_at": lastCheckedAt, "last_error": syncLastError,
		},
	}
	if err := tx.Rollback(ctx); err != nil {
		h.Log.Warn("timetable snapshot rollback", "error", err)
	}
	writeJSON(w, http.StatusOK, resp)
}

// AcademicTx runs a transaction; callers that need the resulting counters use
// AcademicTxVersions.
func (h *Academic) AcademicTx(ctx context.Context, owner, semester string, fn func(*store.Queries) (bool, error)) error {
	_, err := h.AcademicTxVersions(ctx, owner, semester, fn)
	return err
}

// AcademicTxVersions runs a transaction and returns the post-commit counters of
// the semester, read under the same lock.
func (h *Academic) AcademicTxVersions(ctx context.Context, owner, semester string, fn func(*store.Queries) (bool, error)) (store.AcademicVersions, error) {
	svc := &academic.Service{Q: h.Q, Pool: h.Pool, Log: h.Log}
	return svc.TxWithVersions(ctx, owner, []string{semester}, academic.TxOptions{BumpTimetable: true}, fn)
}

// writeMutationError maps academic transaction errors for non-course handlers.
func (h *Academic) writeMutationError(w http.ResponseWriter, err error) {
	var ae *academicErr
	if errors.As(err, &ae) {
		writeErrorCode(w, ae.status, ae.message, ae.code, ae.details)
		return
	}
	var ve *academic.ValidationError
	if errors.As(err, &ve) {
		writeErrorCode(w, http.StatusBadRequest, ve.Message, ve.Code, ve.Details)
		return
	}
	if errors.Is(err, academic.ErrUnsupportedDatedSession) {
		writeErrorCode(w, http.StatusUnprocessableEntity, "该安排含真实日期，旧课程模型无法表达", "UNSUPPORTED_DATED_SESSION", nil)
		return
	}
	h.Log.Error("academic mutation failed", "error", err)
	writeErrorCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
}

func applyOverridePatch(ov academic.Overrides, set map[string]any, reset []string) (academic.Overrides, error) {
	seen := map[string]bool{}
	for _, field := range reset {
		switch field {
		case "name", "teacher", "location", "color", "hidden":
			if seen[field] {
				return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: "reset 字段重复：" + field}
			}
			if _, both := set[field]; both {
				return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: "同一字段不能同时 set 和 reset：" + field}
			}
			seen[field] = true
			ov.SetFields = dropField(ov.SetFields, field)
			switch field {
			case "name":
				ov.Name = nil
			case "teacher":
				ov.Teacher = nil
			case "location":
				ov.Location = nil
			case "color":
				ov.Color = nil
			case "hidden":
				ov.Hidden = nil
			}
		default:
			return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: "不支持的 reset 字段：" + field}
		}
	}
	for key, value := range set {
		switch key {
		case "name":
			s, ok := value.(string)
			if !ok || strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > 256 {
				return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: "name 必须是非空字符串（≤256 字符）"}
			}
			ov.SetFields = withField(ov.SetFields, "name")
			ov.Name = &s
		case "teacher", "location", "color":
			s, ok := value.(string)
			if !ok {
				return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: key + " 必须是字符串；清除请用 reset_fields"}
			}
			if key != "color" && utf8.RuneCountInString(s) > 512 {
				return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: key + " 过长（≤512 字符）"}
			}
			if key == "color" && !isHexColor(s) {
				return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: "color 必须是 #RRGGBB"}
			}
			ov.SetFields = withField(ov.SetFields, key)
			switch key {
			case "teacher":
				ov.Teacher = &s
			case "location":
				ov.Location = &s
			case "color":
				ov.Color = &s
			}
		case "hidden":
			b, ok := value.(bool)
			if !ok {
				return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: "hidden 必须是布尔值"}
			}
			ov.SetFields = withField(ov.SetFields, "hidden")
			ov.Hidden = &b
		default:
			return ov, &academic.ValidationError{Code: "INVALID_ARGUMENT", Message: "不支持的覆盖字段：" + key}
		}
	}
	return ov, nil
}

func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func sameOverrides(a, b academic.Overrides) bool {
	if len(a.SetFields) != len(b.SetFields) {
		return false
	}
	set := map[string]bool{}
	for _, f := range a.SetFields {
		set[f] = true
	}
	for _, f := range b.SetFields {
		if !set[f] {
			return false
		}
	}
	return samePtr(a.Name, b.Name) && samePtr(a.Teacher, b.Teacher) &&
		samePtr(a.Location, b.Location) && samePtr(a.Color, b.Color) &&
		sameBoolPtr(a.Hidden, b.Hidden)
}

func samePtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func hashCommitRequest(previewID string, expected int64, acks []string) (string, error) {
	normalized := struct {
		Preview  string   `json:"preview"`
		Expected int64    `json:"expected"`
		Ack      []string `json:"ack"`
	}{Preview: previewID, Expected: expected, Ack: acks}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// writeAliasError reports alias persistence failures without claiming the
// original commit rolled back.
func (h *Academic) writeAliasError(w http.ResponseWriter, err error) {
	if errors.Is(err, errAliasConflict) {
		writeErrorCode(w, http.StatusConflict, "该幂等键已用于另一个请求", "IDEMPOTENCY_CONFLICT", nil)
		return
	}
	h.Log.Error("persist commit alias", "error", err)
	writeErrorCode(w, http.StatusInternalServerError,
		"原提交已成功，但幂等别名未登记成功；请用原幂等键重试", "INTERNAL_ERROR", nil)
}

func normalizedAckCodes(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range in {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// validateWarningAcks requires the acknowledged set to equal the preview's
// WARNING code set: missing and extra codes are both rejected.
func validateWarningAcks(issues []henu.Issue, acked []string) (missing, extra []string) {
	warn := map[string]bool{}
	for _, issue := range issues {
		if issue.Severity == henu.SeverityWarning {
			warn[issue.Code] = true
		}
	}
	ack := map[string]bool{}
	for _, c := range normalizedAckCodes(acked) {
		ack[c] = true
		if !warn[c] {
			extra = append(extra, c)
		}
	}
	for c := range warn {
		if !ack[c] {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

var errAliasConflict = errors.New("idempotency alias conflict")

// registerCommitAlias binds a new idempotency key to an already committed
// preview. It must succeed before the caller reports success; a unique-key race
// is resolved by re-reading the winner.
func (h *Academic) registerCommitAlias(ctx context.Context, owner, key, requestSHA, commitID string, receipt []byte) ([]byte, error) {
	id, err := academic.NewUUID()
	if err != nil {
		return nil, err
	}
	if err := h.Q.InsertCommitRequest(ctx, id, owner, key, requestSHA, commitID, receipt, h.now().Add(30*24*time.Hour)); err == nil {
		return receipt, nil
	} else {
		_, existingSHA, existing, gerr := h.Q.GetCommitRequestByKey(ctx, owner, key)
		if gerr == nil {
			if existingSHA == requestSHA {
				return existing, nil
			}
			return nil, errAliasConflict
		}
		return nil, err
	}
}
