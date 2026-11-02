package dto

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const chinaOffset = 8 * 3600

// RemindAtString formats a UTC timestamptz as the frontend's local (+08:00)
// "YYYY-MM-DD HH:MM" string, or "" when invalid.
func RemindAtString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.In(time.FixedZone("CST", chinaOffset)).Format("2006-01-02 15:04")
}

// ParseDate parses an optional YYYY-MM-DD string into a pgtype.Date.
func ParseDate(s *string) (pgtype.Date, error) {
	if s == nil || *s == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("invalid date %q, want YYYY-MM-DD", *s)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// ParseClock parses an optional HH:MM string into a pgtype.Time.
func ParseClock(s *string) (pgtype.Time, error) {
	if s == nil || *s == "" {
		return pgtype.Time{}, nil
	}
	t, err := time.Parse("15:04", *s)
	if err != nil {
		return pgtype.Time{}, fmt.Errorf("invalid time %q, want HH:MM", *s)
	}
	return pgtype.Time{Microseconds: int64((t.Hour()*3600 + t.Minute()*60)) * 1e6, Valid: true}, nil
}

// ParseRemindAt parses an optional local (+08:00) "YYYY-MM-DD HH:MM" string
// into a UTC pgtype.Timestamptz.
func ParseRemindAt(s *string) (pgtype.Timestamptz, error) {
	if s == nil || *s == "" {
		return pgtype.Timestamptz{}, nil
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", *s, time.FixedZone("CST", chinaOffset))
	if err != nil {
		return pgtype.Timestamptz{}, fmt.Errorf("invalid remind_at %q, want YYYY-MM-DD HH:MM", *s)
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, nil
}

// ParseTS parses an optional RFC3339 string into a pgtype.Timestamptz.
func ParseTS(s *string) (pgtype.Timestamptz, error) {
	if s == nil || *s == "" {
		return pgtype.Timestamptz{}, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return pgtype.Timestamptz{}, fmt.Errorf("invalid timestamp %q, want RFC3339", *s)
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, nil
}

// StrPtr returns a pointer to s.
func StrPtr(s string) *string { return &s }

// Int4Ptr returns a pointer to v, or nil when the pgtype value is invalid.
func Int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	return &v.Int32
}

// Int8Ptr returns a pointer to v, or nil when the pgtype value is invalid.
func Int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
