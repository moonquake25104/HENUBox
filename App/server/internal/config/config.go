// Package config loads the Kairos API server configuration from the
// environment. It supports both APP_* and KAIROS_* prefixes (KAIROS_*
// wins when both are set) plus a few bare names (DATABASE_URL, JWT_SECRET).
//
// A minimal .env file at the repo root is supported via LoadEnv; it is a
// tiny parser (KEY=VALUE lines, # comments) and deliberately avoids a heavy
// dotenv dependency.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Config holds every runtime setting the API server needs.
type Config struct {
	// DBURL is the PostgreSQL connection string.
	DBURL string
	// JWTSecret signs HS256 tokens. Guaranteed >= 32 bytes after Load.
	JWTSecret string
	// JWTTTL is the token lifetime.
	JWTTTL time.Duration
	// Port is the HTTP listen port.
	Port string
	// BindHost is local-only by default; containers opt into 0.0.0.0.
	BindHost string

	// Username is the single account username.
	Username string
	// PasswordHash is the bcrypt hash of the account password.
	PasswordHash string
	// DataDir is the server-side directory for the AI sync DEK and API-key
	// key files.
	DataDir string

	// SMTP settings for the notification mailer. All are optional: an empty
	// SMTPHost disables email delivery (the scheduler only logs warnings).
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string
	SMTPTo   string
	SMTPTLS  string
}

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	cfg := &Config{
		DBURL:     getenv("DATABASE_URL", "KAIROS_DB_URL", "APP_DB_URL", "postgres://kairos:kairos_dev@localhost:5432/kairos_dev"),
		JWTSecret: getenv("JWT_SECRET", "KAIROS_JWT_SECRET", "APP_JWT_SECRET", ""),
		JWTTTL:    durationEnv([]string{"KAIROS_JWT_TTL", "APP_JWT_TTL"}, 7*24*time.Hour),
		Port:      getenv("KAIROS_API_PORT", "APP_API_PORT", "8080"),
		BindHost:  getenv("KAIROS_BIND_HOST", "APP_BIND_HOST", "127.0.0.1"),
		Username:  getenv("APP_USER", "KAIROS_USER", ""),
		DataDir:   getenv("KAIROS_DATA_DIR", "APP_DATA_DIR", "./data"),

		SMTPHost: getenv("SMTP_HOST", "KAIROS_SMTP_HOST", ""),
		SMTPPort: intEnv([]string{"SMTP_PORT", "KAIROS_SMTP_PORT"}, 587),
		SMTPUser: getenv("SMTP_USER", "KAIROS_SMTP_USER", ""),
		SMTPPass: getenv("SMTP_PASS", "KAIROS_SMTP_PASS", ""),
		SMTPFrom: getenv("SMTP_FROM", "KAIROS_SMTP_FROM", "Kairos <no-reply@kairos.local>"),
		SMTPTo:   getenv("SMTP_TO", "KAIROS_SMTP_TO", ""),
		SMTPTLS:  getenv("SMTP_TLS", "KAIROS_SMTP_TLS", "starttls"),
	}

	hash := getenv("APP_PASSWORD_HASH", "KAIROS_PASSWORD_HASH", "")
	if plain := getenv("APP_PASSWORD", "KAIROS_PASSWORD", ""); plain != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash APP_PASSWORD: %w", err)
		}
		hash = string(h)
	}
	cfg.PasswordHash = hash

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes, got %d", len(c.JWTSecret))
	}
	if c.Username == "" {
		return fmt.Errorf("APP_USER is required")
	}
	if c.PasswordHash == "" {
		return fmt.Errorf("APP_PASSWORD_HASH (or APP_PASSWORD) is required")
	}
	if _, err := bcrypt.Cost([]byte(c.PasswordHash)); err != nil {
		return fmt.Errorf("APP_PASSWORD_HASH is not a valid bcrypt hash: %w", err)
	}
	if c.DBURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.Port == "" {
		return fmt.Errorf("KAIROS_API_PORT is required")
	}
	return nil
}

// getenv returns the first non-empty value among keys, else fallback.
func getenv(keys ...string) string {
	for _, k := range keys[:len(keys)-1] {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return keys[len(keys)-1]
}

func durationEnv(keys []string, fallback time.Duration) time.Duration {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				return d
			}
		}
	}
	return fallback
}

// intEnv returns the first parseable integer among keys, else fallback.
func intEnv(keys []string, fallback int) int {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
	}
	return fallback
}

// LoadEnv reads a simple KEY=VALUE .env file (if present) into the process
// environment without overwriting already-set variables. It is best-effort:
// a missing file is not an error. Candidates are tried in order so the file
// is found whether the process runs from the repo root or from server/.
func LoadEnv(candidates ...string) error {
	for _, path := range candidates {
		if err := loadEnvFile(path); err != nil {
			return err
		}
	}
	return nil
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}
