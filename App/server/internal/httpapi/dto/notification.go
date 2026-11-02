package dto

import (
	"kairos/server/internal/store"
)

// NotificationConfig mirrors the frontend src/types/notification.ts. The
// exam_offsets_json and course_offsets_json fields carry the DB jsonb bytes
// as JSON array strings.
type NotificationConfig struct {
	ID                    int64  `json:"id"`
	Enabled               bool   `json:"enabled"`
	ExamOffsetsJSON       string `json:"exam_offsets_json"`
	CourseOffsetsJSON     string `json:"course_offsets_json"`
	AndroidChannelCreated bool   `json:"android_channel_created"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}

// UpdateNotificationConfigRequest is the body of PATCH /api/notify/config.
// Omitted fields keep their stored value.
type UpdateNotificationConfigRequest struct {
	Enabled               *bool   `json:"enabled"`
	ExamOffsetsJSON       *string `json:"exam_offsets_json"`
	CourseOffsetsJSON     *string `json:"course_offsets_json"`
	AndroidChannelCreated *bool   `json:"android_channel_created"`
}

// FromNotificationConfig converts a store row into the API representation.
// android_channel_created is always false: the web build has no Android
// notification channel and the value is never persisted.
func FromNotificationConfig(c store.NotificationConfig) NotificationConfig {
	return NotificationConfig{
		ID:                    c.ID,
		Enabled:               c.Enabled,
		ExamOffsetsJSON:       string(c.ExamOffsets),
		CourseOffsetsJSON:     string(c.CourseOffsets),
		AndroidChannelCreated: false,
		CreatedAt:             store.TSString(c.CreatedAt),
		UpdatedAt:             store.TSString(c.UpdatedAt),
	}
}
