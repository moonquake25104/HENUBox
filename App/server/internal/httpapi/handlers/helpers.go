package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// decodeJSON decodes the request body into v, returning a 400 on malformed JSON.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// pathID parses the {id} route parameter as an int64.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// isNotFound reports whether err is a pgx no-rows error.
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// writeStoreError maps a store error to an HTTP response.
func writeStoreError(w http.ResponseWriter, err error) {
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal server error")
}

// errorEnvelope is the compatibility envelope: `error` stays a string for the
// existing frontend, while `code` and `details` are additive.
type errorEnvelope struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details any    `json:"details,omitempty"`
}

// writeErrorCode writes the error envelope with a stable machine-readable code.
func writeErrorCode(w http.ResponseWriter, status int, message, code string, details any) {
	writeJSON(w, status, errorEnvelope{Error: message, Code: code, Details: details})
}

// chiURLParam reads a named path parameter.
func chiURLParam(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}

// writeRawJSON writes an already-serialized JSON payload unchanged.
func writeRawJSON(w http.ResponseWriter, status int, raw []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
