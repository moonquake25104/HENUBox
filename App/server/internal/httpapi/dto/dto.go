// Package dto holds the request/response structures for the Kairos API.
// Field names use snake_case JSON to match the frontend src/types conventions.
package dto

// LoginRequest is the body of POST /api/auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// User is the public representation of the single account.
type User struct {
	Username string `json:"username"`
}

// LoginResponse is the body of a successful login.
type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// MeResponse is the body of GET /api/auth/me.
type MeResponse struct {
	Username string `json:"username"`
}

// ErrorResponse is a uniform error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
