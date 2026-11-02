package dto

import (
	"kairos/server/internal/store"
)

// SyncConfig mirrors the frontend src/types/sync.ts SyncConfig. The password
// is never returned; only password_configured is exposed.
type SyncConfig struct {
	ID                 int64   `json:"id"`
	ServerURL          string  `json:"server_url"`
	Username           string  `json:"username"`
	AutoSync           bool    `json:"auto_sync"`
	LastSyncAt         *string `json:"last_sync_at"`
	RemoteEtag         *string `json:"remote_etag"`
	DeviceID           *string `json:"device_id"`
	DatasetID          *string `json:"dataset_id"`
	PasswordConfigured bool    `json:"password_configured"`
}

// UpdateSyncConfigRequest is the body of PATCH /api/sync/config. An omitted
// password keeps the stored one; an empty string clears it.
type UpdateSyncConfigRequest struct {
	ServerURL string  `json:"server_url"`
	Username  string  `json:"username"`
	AutoSync  bool    `json:"auto_sync"`
	Password  *string `json:"password"`
}

// SyncStats mirrors the frontend src/types/sync.ts SyncStats.
type SyncStats struct {
	TasksMerged      int `json:"tasks_merged"`
	CoursesMerged    int `json:"courses_merged"`
	ExamsMerged      int `json:"exams_merged"`
	SessionsMerged   int `json:"sessions_merged"`
	TermPhasesMerged int `json:"term_phases_merged"`
	Conflicts        int `json:"conflicts"`
}

// SyncResult mirrors the frontend src/types/sync.ts SyncResult.
type SyncResult struct {
	Uploaded   bool      `json:"uploaded"`
	Downloaded bool      `json:"downloaded"`
	Stats      SyncStats `json:"stats"`
}

// RecoveryKeyResponse is the body of GET /api/sync/ai-recovery-key.
type RecoveryKeyResponse struct {
	RecoveryKey *string `json:"recovery_key"`
}

// SetRecoveryKeyRequest is the body of POST /api/sync/ai-recovery-key.
type SetRecoveryKeyRequest struct {
	RecoveryKey string `json:"recovery_key"`
}

// FromSyncConfig converts a store config row into the API representation.
func FromSyncConfig(c store.SyncConfig) SyncConfig {
	return SyncConfig{
		ID:                 c.ID,
		ServerURL:          c.ServerUrl,
		Username:           c.Username,
		AutoSync:           c.AutoSync,
		LastSyncAt:         tsPtr(c.LastSyncAt),
		RemoteEtag:         textPtr(c.RemoteEtag),
		DeviceID:           textPtr(c.DeviceID),
		DatasetID:          textPtr(c.DatasetID),
		PasswordConfigured: c.Password != "",
	}
}
