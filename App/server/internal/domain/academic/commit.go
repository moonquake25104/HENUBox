package academic

import (
	"context"
	"fmt"
	"time"

	"kairos/server/internal/store"
)

// CommitCounts is the effect summary stored in the batch receipt.
type CommitCounts struct {
	CoursesCreated       int `json:"courses_created"`
	CoursesUpdated       int `json:"courses_updated"`
	SessionsCreated      int `json:"sessions_created"`
	SessionsUpdated      int `json:"sessions_updated"`
	SessionsRetired      int `json:"sessions_retired"`
	Unchanged            int `json:"unchanged"`
	Retained             int `json:"retained"`
	LegacyCoursesCreated int `json:"legacy_courses_created"`
	LegacyCoursesUpdated int `json:"legacy_courses_updated"`
}

// CommitResult is the receipt returned by commit and by GET /commits/{id}.
type CommitResult struct {
	CommitID               string       `json:"commit_id"`
	PreviewID              string       `json:"preview_id"`
	Semester               string       `json:"semester"`
	Status                 string       `json:"status"`
	CommittedAt            string       `json:"committed_at"`
	Revision               int64        `json:"revision"`
	AggregateRevision      int64        `json:"aggregate_revision"`
	LegacyCourseIDs        []int64      `json:"legacy_course_ids"`
	ReminderRecomputeState string       `json:"reminder_recompute_state"`
	Counts                 CommitCounts `json:"counts"`
	WarningsAcknowledged   []string     `json:"warnings_acknowledged"`
	NoChanges              bool         `json:"no_changes"`
	Replayed               bool         `json:"replayed"`
}

// CommitInput is everything needed to apply one preview inside a transaction.
type CommitInput struct {
	Owner    string
	Semester string
	CommitID string
	BatchID  string
	Preview  *Preview
	Plan     *Plan
	Settings Settings
	Now      time.Time
}

// CommitImport applies the preview's plan: source identities, immutable
// revisions, legacy projections and retirement decisions. The caller owns the
// transaction and the version bump.
func CommitImport(ctx context.Context, q *store.Queries, in CommitInput) (CommitCounts, error) {
	counts := CommitCounts{}
	courseIDs := map[string]string{}

	for _, pc := range in.Preview.Courses {
		id := in.Plan.CourseMatches[pc.PreviewCourseID]
		if id == "" {
			newID, err := NewUUID()
			if err != nil {
				return counts, err
			}
			if err := q.InsertAcademicCourse(ctx, newID, in.Owner, in.Semester, "henu", pc.Name,
				pc.SourceIdentity.UpstreamCourseCode, pc.SourceIdentity.UpstreamTeachingClassCode,
				pc.Credits, &in.BatchID); err != nil {
				return counts, err
			}
			id = newID
			counts.CoursesCreated++
		} else {
			if err := q.UpdateAcademicCourseMetadata(ctx, id, pc.Name, pc.Credits,
				pc.SourceIdentity.UpstreamCourseCode, pc.SourceIdentity.UpstreamTeachingClassCode, in.BatchID); err != nil {
				return counts, err
			}
			counts.CoursesUpdated++
		}
		courseIDs[pc.PreviewCourseID] = id

		for _, ps := range pc.Sessions {
			values := Values{
				Name: pc.Name, Weekday: ps.Weekday, Sections: ps.Sections, Weeks: ps.Weeks,
				Location: ps.Location, Teacher: ps.Teacher, Date: ps.Date,
			}
			hash := SessionContentHash(values)

			sessionID := ""
			if in.Plan.SessionMatches[ps.PreviewSessionID] != "" {
				sessionID = in.Plan.SessionMatches[ps.PreviewSessionID]
			} else if ps.SessionID != nil {
				sessionID = *ps.SessionID
			}

			if sessionID != "" {
				row, err := q.GetSourceSessionByID(ctx, sessionID)
				if err != nil {
					return counts, fmt.Errorf("load existing session: %w", err)
				}
				if row.ContentSHA256 != hash {
					revID, err := NewUUID()
					if err != nil {
						return counts, err
					}
					if err := q.InsertSourceRevision(ctx, revID, row.SourceEntryID, &in.BatchID, hash, mustJSON(values)); err != nil {
						return counts, err
					}
					counts.SessionsUpdated++
				}
				if err := q.SetSessionRetired(ctx, sessionID, false); err != nil {
					return counts, err
				}
			} else {
				entryID, err := NewUUID()
				if err != nil {
					return counts, err
				}
				revID, err := NewUUID()
				if err != nil {
					return counts, err
				}
				sessionID, err = NewUUID()
				if err != nil {
					return counts, err
				}
				if err := q.InsertSourceEntry(ctx, entryID, in.Owner, in.Semester, "henu", courseIDs[pc.PreviewCourseID], nil); err != nil {
					return counts, err
				}
				if err := q.InsertSourceRevision(ctx, revID, entryID, &in.BatchID, hash, mustJSON(values)); err != nil {
					return counts, err
				}
				if err := q.InsertAcademicSession(ctx, sessionID, courseIDs[pc.PreviewCourseID], entryID); err != nil {
					return counts, err
				}
				counts.SessionsCreated++
			}

			created, err := projectOne(ctx, q, in.Owner, sessionID, in.Semester)
			if err != nil {
				return counts, err
			}
			if created {
				counts.LegacyCoursesCreated++
			} else {
				counts.LegacyCoursesUpdated++
			}
		}
	}

	for _, m := range in.Preview.Diff.Missing {
		if m.Decision == nil || *m.Decision != DecisionRetire {
			continue
		}
		if err := q.SetSessionRetired(ctx, m.SessionID, true); err != nil {
			return counts, err
		}
		if _, err := projectOne(ctx, q, in.Owner, m.SessionID, in.Semester); err != nil {
			return counts, err
		}
		retired, err := q.GetSourceSessionByID(ctx, m.SessionID)
		if err != nil {
			return counts, err
		}
		if retired.LegacyCourseID != nil {
			if err := q.SoftDeleteCourse(ctx, *retired.LegacyCourseID); err != nil {
				return counts, err
			}
		}
		counts.SessionsRetired++
	}
	return counts, nil
}

// projectOne writes the effective values into the legacy course, creating the
// row and link when the session has never been projected.
func projectOne(ctx context.Context, q *store.Queries, owner, sessionID, semester string) (created bool, err error) {
	row, err := q.GetSourceSessionByID(ctx, sessionID)
	if err != nil {
		return false, err
	}
	proj, err := ProjectSession(ctx, q, owner, row)
	if err != nil {
		return false, err
	}
	if row.LegacyCourseID != nil {
		return false, nil
	}
	course, err := q.CreateCourse(ctx, store.CreateCourseParams{
		Name:              proj.Name,
		DayOfWeek:         proj.DayOfWeek,
		StartTime:         store.ClockOf(proj.StartTime),
		EndTime:           store.ClockOf(proj.EndTime),
		WeekPattern:       proj.WeekPattern,
		SemesterStartDate: store.DateOf(proj.SemesterStartDate),
		Location:          proj.Location,
		Teacher:           proj.Teacher,
		Color:             proj.Color,
		Semester:          semester,
	})
	if err != nil {
		return false, err
	}
	if err := q.UpsertLegacyLink(ctx, sessionID, course.ID); err != nil {
		return false, err
	}
	if proj.Hidden {
		if err := q.SoftDeleteCourse(ctx, course.ID); err != nil {
			return false, err
		}
	}
	return true, nil
}
