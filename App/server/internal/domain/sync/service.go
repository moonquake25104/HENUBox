package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// ErrNotConfigured is returned when no WebDAV server URL is configured.
var ErrNotConfigured = errors.New("未配置服务器地址")

// ErrAcademicSyncUnsupported is returned when generic sync would modify legacy
// courses on a deployment that already has academic source links.
var ErrAcademicSyncUnsupported = errors.New("已有河大来源课表，通用同步不支持修改课程")

// TxBeginner is satisfied by *pgxpool.Pool and *pgx.Conn.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Service orchestrates a sync_now run: pull → LWW merge → conditional push,
// plus the non-fatal AI settings envelope sync. No background auto-sync
// goroutine is spawned; sync is strictly request/response.
type Service struct {
	Q       *store.Queries
	Pool    TxBeginner
	DataDir string
	// Owner is the application account that owns the academic version rows.
	Owner string
}

// TestConnection checks the configured WebDAV server reachability.
func (s *Service) TestConnection(ctx context.Context) (bool, error) {
	cfg, err := s.Q.GetSyncConfig(ctx)
	if err != nil {
		return false, err
	}
	if cfg.ServerUrl == "" {
		return false, ErrNotConfigured
	}
	client := NewClient(cfg.ServerUrl, cfg.Username, cfg.Password)
	return client.TestConnection(ctx)
}

// SyncNow runs the full sync cycle and returns the merge summary.
func (s *Service) SyncNow(ctx context.Context) (*SyncResult, error) {
	cfg, err := s.Q.GetSyncConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.ServerUrl == "" {
		return nil, ErrNotConfigured
	}
	client := NewClient(cfg.ServerUrl, cfg.Username, cfg.Password)

	stats := SyncStats{}
	var remoteEtag *string
	downloaded := false

	remote, err := client.Download(ctx)
	if err == nil {
		remoteEtag = remote.Etag
		stats, err = s.importAll(ctx, remote.Data)
		if err != nil {
			return nil, err
		}
		downloaded = true
	} else if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("下载失败：%w", err)
	}

	exportedAt, etag, retryStats, err := s.uploadSnapshot(ctx, client, remoteEtag)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			exportedAt, etag, retryStats, err = s.retryAfterConflict(ctx, client)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("上传失败：%w", err)
		}
	}
	addStats(&stats, retryStats)

	if err := s.syncAiSettings(ctx, client, cfg.Password); err != nil {
		slog.Warn("AI 设置同步失败（已跳过，不影响数据同步）", "error", err)
	}

	if err := s.Q.UpdateSyncLastSyncAt(ctx, store.TSOf(exportedAt)); err != nil {
		return nil, err
	}
	if err := s.Q.UpdateSyncRemoteEtag(ctx, textOfPtr(etag)); err != nil {
		return nil, err
	}

	return &SyncResult{Uploaded: true, Downloaded: downloaded, Stats: stats}, nil
}

// GetRecoveryKey returns the DEK hex recovery key, or nil when none exists.
func (s *Service) GetRecoveryKey(ctx context.Context) (*string, error) {
	dek, err := loadDEK(s.DataDir)
	if err != nil {
		return nil, err
	}
	if dek == nil {
		return nil, nil
	}
	key := RecoveryKeyHex(*dek)
	return &key, nil
}

// SetRecoveryKey validates and persists a DEK recovery key.
func (s *Service) SetRecoveryKey(ctx context.Context, hexKey string) error {
	dek, err := RecoveryKeyFromHex(hexKey)
	if err != nil {
		return err
	}
	return saveDEK(s.DataDir, &dek)
}

func (s *Service) importAll(ctx context.Context, data *SyncData) (SyncStats, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SyncStats{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := s.Q.WithTx(tx)

	// Phase B1 compatibility gate: once any academic source link exists, the
	// generic sync must not create or modify courses. Rejecting the whole sync
	// keeps tasks/sessions/exams consistent with courses.
	if len(data.Courses) > 0 {
		links, err := qtx.CountAcademicSourceSessions(ctx)
		if err != nil {
			return SyncStats{}, err
		}
		if links > 0 {
			changes, err := wouldChangeCourses(ctx, qtx, data.Courses)
			if err != nil {
				return SyncStats{}, err
			}
			if changes {
				return SyncStats{}, ErrAcademicSyncUnsupported
			}
		}
	}

	stats, err := ImportAll(ctx, qtx, data)
	if err != nil {
		return SyncStats{}, err
	}
	if stats.CoursesMerged > 0 {
		if err := bumpCourseSemesters(ctx, qtx, s.Owner, data.Courses); err != nil {
			return SyncStats{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncStats{}, err
	}
	return stats, nil
}

// wouldChangeCourses reports whether the incoming snapshot would insert or
// update any legacy course.
func wouldChangeCourses(ctx context.Context, q *store.Queries, remote []Course) (bool, error) {
	for _, c := range remote {
		normalized := normalizeCourse(c)
		local, err := findCourseMeta(ctx, q, normalized.SyncID, normalized.ID, c.SyncID == "")
		if err != nil {
			return false, err
		}
		if local == nil {
			return true, nil
		}
		if effectiveTimestamp(normalized.UpdatedAt, normalized.DeletedAt) > local.effective() {
			return true, nil
		}
	}
	return false, nil
}

// bumpCourseSemesters increments timetable/aggregate once per affected
// partition before the sync transaction commits.
func bumpCourseSemesters(ctx context.Context, q *store.Queries, owner string, courses []Course) error {
	if owner == "" {
		return nil
	}
	seen := map[string]struct{}{}
	for _, c := range courses {
		if _, ok := seen[c.Semester]; ok {
			continue
		}
		seen[c.Semester] = struct{}{}
		if _, err := q.BumpAcademicVersions(ctx, owner, c.Semester, 0, 1, 1); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) uploadSnapshot(ctx context.Context, client *Client, remoteEtag *string) (string, *string, SyncStats, error) {
	data, err := ExportAll(ctx, s.Q)
	if err != nil {
		return "", nil, SyncStats{}, err
	}
	etag, err := client.Upload(ctx, data, remoteEtag)
	if err != nil {
		return "", nil, SyncStats{}, err
	}
	return data.ExportedAt, etag, SyncStats{}, nil
}

// retryAfterConflict re-downloads the remote (fresh ETag), merges it and
// uploads again. A second 412 aborts to avoid an infinite retry loop.
func (s *Service) retryAfterConflict(ctx context.Context, client *Client) (string, *string, SyncStats, error) {
	slog.Warn("Remote sync data changed during upload; retrying once")
	remote, err := client.Download(ctx)
	if err != nil {
		return "", nil, SyncStats{}, fmt.Errorf("冲突后重新下载失败：%w", err)
	}
	retryStats, err := s.importAll(ctx, remote.Data)
	if err != nil {
		return "", nil, SyncStats{}, err
	}
	exportedAt, etag, _, err := s.uploadSnapshot(ctx, client, remote.Etag)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return "", nil, SyncStats{}, errors.New("上传失败：重试期间远程数据再次变化")
		}
		return "", nil, SyncStats{}, fmt.Errorf("上传失败：%w", err)
	}
	return exportedAt, etag, retryStats, nil
}

// syncAiSettings syncs the encrypted AI settings envelope. It is non-fatal:
// failures are logged by the caller and never block the main snapshot sync.
func (s *Service) syncAiSettings(ctx context.Context, client *Client, password string) error {
	if password == "" {
		return nil
	}
	aiCfg, err := s.Q.GetAiConfig(ctx)
	if err != nil {
		return err
	}
	if !aiCfg.SyncEnabled {
		return nil
	}

	localDEK, err := loadDEK(s.DataDir)
	if err != nil {
		return err
	}
	localKey, err := ensureKeyFile(s.DataDir)
	if err != nil {
		return err
	}
	localAPIKey := ""
	if aiCfg.ApiKeyEncrypted != "" {
		localAPIKey, err = decryptAPIKey(aiCfg.ApiKeyEncrypted, localKey)
		if err != nil {
			return fmt.Errorf("解密本地 API 密钥失败：%w", err)
		}
	}
	localPayload := PayloadFromConfig(aiCfg, localAPIKey)

	remote, err := client.DownloadAiSettings(ctx)
	hadRemote := true
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			hadRemote = false
		} else {
			return err
		}
	}
	var uploadEtag *string
	if hadRemote {
		uploadEtag = remote.Etag
	}

	var mergedPayload SyncAiPayload
	var mergedDEK [32]byte
	if hadRemote {
		decrypted, err := DecryptBlob(password, remote.Blob, localDEK)
		if err != nil {
			return err
		}
		switch MergePayload(&localPayload, &decrypted.Payload) {
		case RemoteWins:
			reencrypted := ""
			if decrypted.Payload.APIKey != "" {
				reencrypted, err = encryptAPIKey(decrypted.Payload.APIKey, localKey)
				if err != nil {
					return err
				}
			}
			if err := s.Q.ApplySyncedAiConfig(ctx, store.ApplySyncedAiConfigParams{
				Enabled:         decrypted.Payload.Enabled,
				BaseUrl:         decrypted.Payload.BaseURL,
				Model:           decrypted.Payload.Model,
				ApiKeyEncrypted: reencrypted,
				UpdatedAt:       store.TSOf(decrypted.Payload.UpdatedAt),
			}); err != nil {
				return err
			}
			mergedPayload = decrypted.Payload
			mergedDEK = decrypted.DEK
		case LocalWins:
			mergedPayload = localPayload
			mergedDEK = decrypted.DEK
		}
	} else {
		mergedPayload = localPayload
		if localDEK != nil {
			mergedDEK = *localDEK
		}
	}

	finalDEK := mergedDEK
	if !hadRemote && localDEK == nil {
		finalDEK, err = loadOrCreateDEK(s.DataDir)
		if err != nil {
			return err
		}
	}

	newBlob, err := EncryptToBlob(password, &mergedPayload, finalDEK)
	if err != nil {
		return err
	}
	newEtag, err := client.UploadAiSettings(ctx, newBlob, uploadEtag)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			newEtag, err = s.retryAiSettingsUpload(ctx, client, password, finalDEK)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if localDEK == nil {
		if err := saveDEK(s.DataDir, &finalDEK); err != nil {
			return err
		}
	}
	return s.Q.UpdateSyncAiSettingsRemoteEtag(ctx, textOfPtr(newEtag))
}

func (s *Service) retryAiSettingsUpload(ctx context.Context, client *Client, password string, dek [32]byte) (*string, error) {
	slog.Warn("AI 设置上传冲突，重试一次")
	latest, err := client.DownloadAiSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("冲突后重新下载 AI 设置失败：%w", err)
	}
	decrypted, err := DecryptBlob(password, latest.Blob, &dek)
	if err != nil {
		return nil, err
	}
	cfg, err := s.Q.GetAiConfig(ctx)
	if err != nil {
		return nil, err
	}
	key, err := ensureKeyFile(s.DataDir)
	if err != nil {
		return nil, err
	}
	apiKey := ""
	if cfg.ApiKeyEncrypted != "" {
		apiKey, err = decryptAPIKey(cfg.ApiKeyEncrypted, key)
		if err != nil {
			return nil, err
		}
	}
	localPayload := PayloadFromConfig(cfg, apiKey)
	winner := localPayload
	if MergePayload(&localPayload, &decrypted.Payload) == RemoteWins {
		winner = decrypted.Payload
	}
	retryBlob, err := EncryptToBlob(password, &winner, decrypted.DEK)
	if err != nil {
		return nil, err
	}
	etag, err := client.UploadAiSettings(ctx, retryBlob, latest.Etag)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return nil, errors.New("AI 设置上传失败：重试期间远端再次变化")
		}
		return nil, fmt.Errorf("AI 设置上传失败：%w", err)
	}
	return etag, nil
}

func addStats(target *SyncStats, incoming SyncStats) {
	target.TasksMerged += incoming.TasksMerged
	target.CoursesMerged += incoming.CoursesMerged
	target.ExamsMerged += incoming.ExamsMerged
	target.SessionsMerged += incoming.SessionsMerged
	target.TermPhasesMerged += incoming.TermPhasesMerged
	target.Conflicts += incoming.Conflicts
}

func textOfPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return store.TextOf(*v)
}
