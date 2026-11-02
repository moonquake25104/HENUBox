package store

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// DateOf builds a valid pgtype.Date from a YYYY-MM-DD string.
func DateOf(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// ClockOf builds a valid pgtype.Time from an HH:MM string.
func ClockOf(s string) pgtype.Time {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return pgtype.Time{}
	}
	return pgtype.Time{Microseconds: int64((t.Hour()*3600 + t.Minute()*60)) * 1e6, Valid: true}
}

// TSOf builds a valid pgtype.Timestamptz from an RFC3339 string.
func TSOf(s string) pgtype.Timestamptz {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// TextOf builds a valid pgtype.Text from a string.
func TextOf(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// Int8Of builds a valid pgtype.Int8 from an int64.
func Int8Of(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: true}
}

// Int4Of builds a valid pgtype.Int4 from an int32.
func Int4Of(v int32) pgtype.Int4 {
	return pgtype.Int4{Int32: v, Valid: true}
}

// DateString formats a pgtype.Date as YYYY-MM-DD, or "" when invalid.
func DateString(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

// ClockString formats a pgtype.Time as HH:MM, or "" when invalid.
func ClockString(t pgtype.Time) string {
	if !t.Valid {
		return ""
	}
	return time.UnixMicro(t.Microseconds).UTC().Format("15:04")
}

// TSString formats a pgtype.Timestamptz as an RFC3339 UTC string, or "" when invalid.
func TSString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// TextString returns the string value of a pgtype.Text, or "" when invalid.
func TextString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}
