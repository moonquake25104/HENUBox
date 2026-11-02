// Package migrate applies the versioned, embed SQL migrations for the
// Kairos PostgreSQL schema. Migrations live in
// kairos/server/internal/store/migrations and are recorded in the
// schema_migrations table so applying them twice is a no-op.
package migrate

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"kairos/server/internal/store/migrations"
)

// DB is satisfied by *pgx.Conn, *pgx.Tx and *pgxpool.Pool.
type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
}

const bootstrapSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    bigint PRIMARY KEY,
    name       text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);`

type migration struct {
	version int64
	name    string
	sql     string
}

// Up applies every pending migration in filename/version order. Each
// migration runs in its own transaction and is recorded in
// schema_migrations; already-applied versions are skipped.
func Up(ctx context.Context, db DB) error {
	if _, err := db.Exec(ctx, bootstrapSQL); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	list, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range list {
		var applied bool
		if err := db.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)",
			m.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if applied {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

func applyOne(ctx context.Context, db DB, m migration) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", m.version, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	for _, stmt := range splitStatements(m.sql) {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migration %d (%s): %w; statement: %.120s", m.version, m.name, err, stmt)
		}
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
		m.version, m.name,
	); err != nil {
		return fmt.Errorf("record migration %d: %w", m.version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %d: %w", m.version, err)
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	var out []migration
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		version, label, ok := parseFileName(name)
		if !ok {
			return nil, fmt.Errorf("migration file %q: want <version>_<name>.up.sql", name)
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", name, err)
		}
		out = append(out, migration{version: version, name: label, sql: string(body)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func parseFileName(name string) (version int64, label string, ok bool) {
	stem := strings.TrimSuffix(name, ".up.sql")
	numPart, labelPart := stem, stem
	if i := strings.IndexByte(stem, '_'); i >= 0 {
		numPart, labelPart = stem[:i], stem[i+1:]
	}
	version, err := strconv.ParseInt(numPart, 10, 64)
	if err != nil || version <= 0 {
		return 0, "", false
	}
	return version, labelPart, true
}

// splitStatements splits a SQL script on top-level semicolons, ignoring
// semicolons inside string literals, quoted identifiers, dollar-quoted
// strings and comments.
func splitStatements(script string) []string {
	var out []string
	var sb strings.Builder
	i := 0
	n := len(script)
	for i < n {
		c := script[i]
		switch {
		case c == '\'' || c == '"':
			sb.WriteByte(c)
			quote := c
			i++
			for i < n {
				if script[i] == '\\' && quote == '\'' && i+1 < n {
					sb.WriteByte('\\')
					i++
					if i < n {
						sb.WriteByte(script[i])
						i++
					}
					continue
				}
				if script[i] == quote {
					if quote == '\'' && i+1 < n && script[i+1] == '\'' {
						sb.WriteString("''")
						i += 2
						continue
					}
					sb.WriteByte(quote)
					i++
					break
				}
				sb.WriteByte(script[i])
				i++
			}
		case c == '-' && i+1 < n && script[i+1] == '-':
			for i < n && script[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && script[i+1] == '*':
			i += 2
			for i+1 < n && !(script[i] == '*' && script[i+1] == '/') {
				i++
			}
			i += 2
		case c == '$':
			if tag := dollarQuoteTag(script[i:]); tag != "" {
				sb.WriteString(tag)
				i += len(tag)
				end := strings.Index(script[i:], tag)
				if end < 0 {
					end = n - i
				}
				sb.WriteString(script[i : i+end])
				i += end + len(tag)
			} else {
				sb.WriteByte(c)
				i++
			}
		case c == ';':
			if trimmed := strings.TrimSpace(sb.String()); trimmed != "" {
				out = append(out, trimmed)
			}
			sb.Reset()
			i++
		default:
			sb.WriteByte(c)
			i++
		}
	}
	if trimmed := strings.TrimSpace(sb.String()); trimmed != "" {
		out = append(out, trimmed)
	}
	return out
}

func dollarQuoteTag(s string) string {
	if len(s) < 2 || s[0] != '$' {
		return ""
	}
	for j := 1; j < len(s); j++ {
		if s[j] == '$' {
			return s[:j+1]
		}
		if s[j] != '_' && (s[j] < 'a' || s[j] > 'z') && (s[j] < 'A' || s[j] > 'Z') && (s[j] < '0' || s[j] > '9') {
			return ""
		}
	}
	return ""
}
