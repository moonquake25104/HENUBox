package middleware

import (
	"net/http"
	"strings"
)

// corsAllowedMethods / corsAllowedHeaders cover the existing API surface:
// PUT saves semester settings; PATCH edits courses/sessions; the rest are the
// standard business verbs. Adding a header here requires a design review.
const (
	corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowedHeaders = "Authorization, Content-Type"
	corsMaxAge         = "600"
)

// CORS returns middleware with an EXACT-match origin allowlist. Empty list
// keeps the historic behavior: no CORS headers at all.
//
// Security rules (phase C mobile design): no wildcard, no Origin reflection;
// a successful preflight never authenticates the actual request; requests
// without an Origin header (same-origin browsers, curl) are untouched.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok {
				// Not an allowed origin: no CORS headers, request proceeds
				// (and fails any auth as usual). The browser blocks reading.
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			// Bearer-token API: no cookies, so no Allow-Credentials.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				w.Header().Set("Access-Control-Max-Age", corsMaxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
