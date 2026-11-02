// Package sync implements the v2 WebDAV snapshot protocol: export/import with
// LWW merge and tombstones, the WebDAV transport, and the AI-settings envelope
// encryption. It is a faithful port of src-tauri/src/sync/.
package sync

// SyncData is the top-level v2 snapshot JSON envelope. schema_version = 2 is
// the current format (sync_id + tombstones); version 1 (no sync_id) is still
// importable. Field names match the Rust serde serialization exactly.
type SyncData struct {
	SchemaVersion    int64             `json:"schema_version"`
	DatasetID        string            `json:"dataset_id"`
	DeviceID         string            `json:"device_id"`
	Tasks            []Task            `json:"tasks"`
	Courses          []Course          `json:"courses"`
	Exams            []Exam            `json:"exams"`
	PomodoroSessions []PomodoroSession `json:"pomodoro_sessions"`
	TermPhases       []TermPhase       `json:"term_phases"`
	ExportedAt       string            `json:"exported_at"`
}

// Task mirrors the Rust db::models::Task snapshot serialization.
type Task struct {
	ID                int64   `json:"id"`
	SyncID            string  `json:"sync_id"`
	Title             string  `json:"title"`
	Description       string  `json:"description"`
	Status            string  `json:"status"`
	Priority          string  `json:"priority"`
	DueDate           *string `json:"due_date"`
	Tags              string  `json:"tags"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	IsDaily           bool    `json:"is_daily"`
	LastCompletedDate *string `json:"last_completed_date"`
	ReminderTime      *string `json:"reminder_time"`
	RemindAt          *string `json:"remind_at"`
	DeletedAt         *string `json:"deleted_at"`
}

// Course mirrors the Rust db::models::Course snapshot serialization.
type Course struct {
	ID                int64   `json:"id"`
	SyncID            string  `json:"sync_id"`
	Name              string  `json:"name"`
	DayOfWeek         int64   `json:"day_of_week"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	WeekPattern       string  `json:"week_pattern"`
	SemesterStartDate string  `json:"semester_start_date"`
	Location          string  `json:"location"`
	Teacher           string  `json:"teacher"`
	Color             string  `json:"color"`
	Semester          string  `json:"semester"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	DeletedAt         *string `json:"deleted_at"`
}

// Exam mirrors the Rust db::models::Exam snapshot serialization. ExamEndDatetime
// is a plain string: empty means "not provided".
type Exam struct {
	ID              int64   `json:"id"`
	SyncID          string  `json:"sync_id"`
	CourseName      string  `json:"course_name"`
	ExamDatetime    string  `json:"exam_datetime"`
	ExamEndDatetime string  `json:"exam_end_datetime"`
	Location        string  `json:"location"`
	Notes           string  `json:"notes"`
	CourseID        *int64  `json:"course_id"`
	Semester        string  `json:"semester"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	DeletedAt       *string `json:"deleted_at"`
}

// PomodoroSession mirrors the Rust db::models::PomodoroSession snapshot
// serialization.
type PomodoroSession struct {
	ID          int64   `json:"id"`
	SyncID      string  `json:"sync_id"`
	StartedAt   string  `json:"started_at"`
	EndedAt     *string `json:"ended_at"`
	SessionType string  `json:"session_type"`
	TaskID      *int64  `json:"task_id"`
	DeletedAt   *string `json:"deleted_at"`
}

// TermPhase mirrors the Rust db::models::TermPhase snapshot serialization.
type TermPhase struct {
	ID                       int64   `json:"id"`
	SyncID                   string  `json:"sync_id"`
	TermLabel                string  `json:"term_label"`
	PhaseType                string  `json:"phase_type"`
	StartWeek                int64   `json:"start_week"`
	EndWeek                  int64   `json:"end_week"`
	AffectsCourses           bool    `json:"affects_courses"`
	AffectsExamNotifications bool    `json:"affects_exam_notifications"`
	PomodoroProfile          string  `json:"pomodoro_profile"`
	NotificationRulesJSON    string  `json:"notification_rules_json"`
	SortOrder                int64   `json:"sort_order"`
	DeletedAt                *string `json:"deleted_at"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
}

// SyncStats describes one sync_now merge result. conflicts counts remote
// entities rejected because the local version is newer or equal — not a
// traditional edit conflict.
type SyncStats struct {
	TasksMerged      int `json:"tasks_merged"`
	CoursesMerged    int `json:"courses_merged"`
	ExamsMerged      int `json:"exams_merged"`
	SessionsMerged   int `json:"sessions_merged"`
	TermPhasesMerged int `json:"term_phases_merged"`
	Conflicts        int `json:"conflicts"`
}

// SyncResult is the sync_now return value.
type SyncResult struct {
	Uploaded   bool      `json:"uploaded"`
	Downloaded bool      `json:"downloaded"`
	Stats      SyncStats `json:"stats"`
}
