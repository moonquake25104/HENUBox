package dto

import (
	"kairos/server/internal/domain/calendar"
)

// NextCourse mirrors the frontend src/types/briefing.ts NextCourse.
type NextCourse struct {
	Title     string `json:"title"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Location  string `json:"location"`
}

// TodayCourses mirrors the frontend TodayCourses.
type TodayCourses struct {
	TodayCount    int64       `json:"today_count"`
	CurrentCourse *NextCourse `json:"current_course"`
	NextCourse    *NextCourse `json:"next_course"`
}

// TaskSpotlight mirrors the frontend TaskSpotlight.
type TaskSpotlight struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Priority string  `json:"priority"`
	DueDate  *string `json:"due_date"`
}

// TodayTasks mirrors the frontend TodayTasks.
type TodayTasks struct {
	OverdueCount         int64           `json:"overdue_count"`
	DueTodayCount        int64           `json:"due_today_count"`
	Spotlight            []TaskSpotlight `json:"spotlight"`
	DailyUnfinishedCount int64           `json:"daily_unfinished_count"`
	DailySpotlight       []TaskSpotlight `json:"daily_spotlight"`
}

// UpcomingExam mirrors the frontend UpcomingExam.
type UpcomingExam struct {
	ID           int64  `json:"id"`
	CourseName   string `json:"course_name"`
	ExamDatetime string `json:"exam_datetime"`
	DaysUntil    int64  `json:"days_until"`
	Location     string `json:"location"`
}

// PomodoroBriefing mirrors the frontend PomodoroBriefing.
type PomodoroBriefing struct {
	IsRunning         bool   `json:"is_running"`
	Phase             string `json:"phase"`
	RemainingSeconds  int64  `json:"remaining_seconds"`
	CompletedSessions int64  `json:"completed_sessions"`
}

// PhaseBriefing mirrors the frontend PhaseBriefing.
type PhaseBriefing struct {
	PhaseType                string  `json:"phase_type"`
	TermLabel                *string `json:"term_label"`
	CurrentWeek              *int64  `json:"current_week"`
	CoursesVisible           bool    `json:"courses_visible"`
	ExamNotificationsEnabled bool    `json:"exam_notifications_enabled"`
	PomodoroProfile          string  `json:"pomodoro_profile"`
}

// TodayBriefingResponse mirrors the frontend TodayBriefingResponse.
type TodayBriefingResponse struct {
	Date         string           `json:"date"`
	WeekdayLabel string           `json:"weekday_label"`
	Courses      TodayCourses     `json:"courses"`
	Tasks        TodayTasks       `json:"tasks"`
	Exam         *UpcomingExam    `json:"exam"`
	Pomodoro     PomodoroBriefing `json:"pomodoro"`
	Phase        PhaseBriefing    `json:"phase"`
}

// FromTodayBriefingResponse converts a domain response into the API representation.
func FromTodayBriefingResponse(resp calendar.TodayBriefingResponse) TodayBriefingResponse {
	spotlight := make([]TaskSpotlight, 0, len(resp.Tasks.Spotlight))
	for _, s := range resp.Tasks.Spotlight {
		spotlight = append(spotlight, TaskSpotlight{
			ID:       s.ID,
			Title:    s.Title,
			Priority: s.Priority,
			DueDate:  s.DueDate,
		})
	}
	dailySpotlight := make([]TaskSpotlight, 0, len(resp.Tasks.DailySpotlight))
	for _, s := range resp.Tasks.DailySpotlight {
		dailySpotlight = append(dailySpotlight, TaskSpotlight{
			ID:       s.ID,
			Title:    s.Title,
			Priority: s.Priority,
			DueDate:  s.DueDate,
		})
	}

	var exam *UpcomingExam
	if resp.Exam != nil {
		exam = &UpcomingExam{
			ID:           resp.Exam.ID,
			CourseName:   resp.Exam.CourseName,
			ExamDatetime: resp.Exam.ExamDatetime,
			DaysUntil:    resp.Exam.DaysUntil,
			Location:     resp.Exam.Location,
		}
	}

	return TodayBriefingResponse{
		Date:         resp.Date,
		WeekdayLabel: resp.WeekdayLabel,
		Courses: TodayCourses{
			TodayCount:    resp.Courses.TodayCount,
			CurrentCourse: fromNextCourse(resp.Courses.CurrentCourse),
			NextCourse:    fromNextCourse(resp.Courses.NextCourse),
		},
		Tasks: TodayTasks{
			OverdueCount:         resp.Tasks.OverdueCount,
			DueTodayCount:        resp.Tasks.DueTodayCount,
			Spotlight:            spotlight,
			DailyUnfinishedCount: resp.Tasks.DailyUnfinishedCount,
			DailySpotlight:       dailySpotlight,
		},
		Exam: exam,
		Pomodoro: PomodoroBriefing{
			IsRunning:         resp.Pomodoro.IsRunning,
			Phase:             resp.Pomodoro.Phase,
			RemainingSeconds:  resp.Pomodoro.RemainingSeconds,
			CompletedSessions: resp.Pomodoro.CompletedSessions,
		},
		Phase: PhaseBriefing{
			PhaseType:                resp.Phase.PhaseType,
			TermLabel:                resp.Phase.TermLabel,
			CurrentWeek:              resp.Phase.CurrentWeek,
			CoursesVisible:           resp.Phase.CoursesVisible,
			ExamNotificationsEnabled: resp.Phase.ExamNotificationsEnabled,
			PomodoroProfile:          resp.Phase.PomodoroProfile,
		},
	}
}

func fromNextCourse(c *calendar.NextCourse) *NextCourse {
	if c == nil {
		return nil
	}
	return &NextCourse{
		Title:     c.Title,
		StartTime: c.StartTime,
		EndTime:   c.EndTime,
		Location:  c.Location,
	}
}
