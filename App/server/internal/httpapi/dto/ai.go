package dto

import (
	"kairos/server/internal/store"
)

// AiConfig mirrors the frontend src/types/ai.ts AiConfig. The API key is never
// returned; only api_key_configured is exposed.
type AiConfig struct {
	ID               int64  `json:"id"`
	Enabled          bool   `json:"enabled"`
	BaseURL          string `json:"base_url"`
	Model            string `json:"model"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	SyncEnabled      bool   `json:"sync_enabled"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// UpdateAiConfigRequest is the body of PATCH /api/ai/config. An omitted api_key
// keeps the stored key; an empty string clears it.
type UpdateAiConfigRequest struct {
	Enabled     *bool   `json:"enabled"`
	BaseURL     *string `json:"base_url"`
	Model       *string `json:"model"`
	APIKey      *string `json:"api_key"`
	SyncEnabled *bool   `json:"sync_enabled"`
}

// AiMorningBrief mirrors the frontend src/types/ai.ts AiMorningBrief.
type AiMorningBrief struct {
	ID          int64  `json:"id"`
	Date        string `json:"date"`
	Markdown    string `json:"markdown"`
	Source      string `json:"source"`
	Model       string `json:"model"`
	GeneratedAt string `json:"generated_at"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// AiStreamChunk is one SSE payload: data: {"delta":"..."}.
type AiStreamChunk struct {
	Delta string `json:"delta"`
}

// FromAiConfig converts a store row into the API representation.
func FromAiConfig(c store.AiConfig) AiConfig {
	return AiConfig{
		ID:               c.ID,
		Enabled:          c.Enabled,
		BaseURL:          c.BaseUrl,
		Model:            c.Model,
		APIKeyConfigured: c.ApiKeyEncrypted != "",
		SyncEnabled:      c.SyncEnabled,
		CreatedAt:        store.TSString(c.CreatedAt),
		UpdatedAt:        store.TSString(c.UpdatedAt),
	}
}

// FromMorningBrief converts a store row into the API representation.
func FromMorningBrief(b store.AiMorningBrief) AiMorningBrief {
	return AiMorningBrief{
		ID:          b.ID,
		Date:        store.DateString(b.Date),
		Markdown:    b.Markdown,
		Source:      b.Source,
		Model:       b.Model,
		GeneratedAt: store.TSString(b.GeneratedAt),
		CreatedAt:   store.TSString(b.CreatedAt),
		UpdatedAt:   store.TSString(b.UpdatedAt),
	}
}
