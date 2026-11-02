// Package handlers implements the HTTP handlers for the Kairos API.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/httpapi/middleware"
)

// Auth holds the dependencies for the auth handlers.
type Auth struct {
	Username     string
	PasswordHash string
	JWTSecret    []byte
	TTL          time.Duration
	Log          *slog.Logger
}

// Login verifies the single account credentials and issues a JWT.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !usernameMatches(req.Username, a.Username) || !checkPassword(req.Password, a.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	now := time.Now()
	claims := &middleware.Claims{
		Username: a.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.TTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(a.JWTSecret)
	if err != nil {
		a.Log.Error("sign token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, dto.LoginResponse{
		Token: signed,
		User:  dto.User{Username: a.Username},
	})
}

// Logout is a no-op for the single-account deployment: the client simply
// discards the token. It exists to keep the API contract stable.
func (a *Auth) Logout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me returns the currently authenticated user.
func (a *Auth) Me(w http.ResponseWriter, r *http.Request) {
	username := middleware.UsernameFrom(r.Context())
	writeJSON(w, http.StatusOK, dto.MeResponse{Username: username})
}

func usernameMatches(got, want string) bool {
	return strings.EqualFold(strings.TrimSpace(got), want)
}

func checkPassword(plain, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, dto.ErrorResponse{Error: msg})
}
