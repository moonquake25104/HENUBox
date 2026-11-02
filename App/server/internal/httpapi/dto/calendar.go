package dto

import (
	"kairos/server/internal/domain/calendar"
)

// WeekScheduleItem mirrors the frontend src/types/schedule.ts WeekScheduleItem.
type WeekScheduleItem struct {
	Kind        string `json:"kind"`
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	DayOfWeek   int64  `json:"day_of_week"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Location    string `json:"location"`
	Teacher     string `json:"teacher"`
	Color       string `json:"color"`
	Notes       string `json:"notes"`
	WeekPattern string `json:"week_pattern"`
	CourseID    *int64 `json:"course_id"`
}

// WeekScheduleResponse mirrors the frontend WeekScheduleResponse.
type WeekScheduleResponse struct {
	WeekIndex         int64              `json:"week_index"`
	Semester          string             `json:"semester"`
	SemesterStartDate string             `json:"semester_start_date"`
	WeekStartDate     string             `json:"week_start_date"`
	WeekEndDate       string             `json:"week_end_date"`
	PhaseType         string             `json:"phase_type"`
	CoursesVisible    bool               `json:"courses_visible"`
	Items             []WeekScheduleItem `json:"items"`
}

// CalendarEvent mirrors the frontend CalendarEvent.
type CalendarEvent struct {
	Kind       string   `json:"kind"`
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	DayOfWeek  int64    `json:"day_of_week"`
	StartTime  string   `json:"start_time"`
	EndTime    string   `json:"end_time"`
	Location   string   `json:"location"`
	Color      string   `json:"color"`
	Tags       []string `json:"tags"`
	SourceLink string   `json:"source_link"`
}

// CalendarWeekResponse mirrors the frontend CalendarWeekResponse.
type CalendarWeekResponse struct {
	WeekIndex         int64           `json:"week_index"`
	Semester          string          `json:"semester"`
	SemesterStartDate string          `json:"semester_start_date"`
	WeekStartDate     string          `json:"week_start_date"`
	WeekEndDate       string          `json:"week_end_date"`
	PhaseType         string          `json:"phase_type"`
	CoursesVisible    bool            `json:"courses_visible"`
	Events            []CalendarEvent `json:"events"`
}

// FromWeekScheduleItem converts a domain item into the API representation.
func FromWeekScheduleItem(item calendar.WeekScheduleItem) WeekScheduleItem {
	return WeekScheduleItem{
		Kind:        item.Kind,
		ID:          item.ID,
		Title:       item.Title,
		DayOfWeek:   item.DayOfWeek,
		StartTime:   item.StartTime,
		EndTime:     item.EndTime,
		Location:    item.Location,
		Teacher:     item.Teacher,
		Color:       item.Color,
		Notes:       item.Notes,
		WeekPattern: item.WeekPattern,
		CourseID:    item.CourseID,
	}
}

// FromWeekScheduleResponse converts a domain response into the API representation.
func FromWeekScheduleResponse(resp calendar.WeekScheduleResponse) WeekScheduleResponse {
	items := make([]WeekScheduleItem, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, FromWeekScheduleItem(item))
	}
	return WeekScheduleResponse{
		WeekIndex:         resp.WeekIndex,
		Semester:          resp.Semester,
		SemesterStartDate: resp.SemesterStartDate,
		WeekStartDate:     resp.WeekStartDate,
		WeekEndDate:       resp.WeekEndDate,
		PhaseType:         resp.PhaseType,
		CoursesVisible:    resp.CoursesVisible,
		Items:             items,
	}
}

// FromCalendarEvent converts a domain event into the API representation.
func FromCalendarEvent(event calendar.CalendarEvent) CalendarEvent {
	return CalendarEvent{
		Kind:       event.Kind,
		ID:         event.ID,
		Title:      event.Title,
		DayOfWeek:  event.DayOfWeek,
		StartTime:  event.StartTime,
		EndTime:    event.EndTime,
		Location:   event.Location,
		Color:      event.Color,
		Tags:       event.Tags,
		SourceLink: event.SourceLink,
	}
}

// FromCalendarWeekResponse converts a domain response into the API representation.
func FromCalendarWeekResponse(resp calendar.CalendarWeekResponse) CalendarWeekResponse {
	events := make([]CalendarEvent, 0, len(resp.Events))
	for _, event := range resp.Events {
		events = append(events, FromCalendarEvent(event))
	}
	return CalendarWeekResponse{
		WeekIndex:         resp.WeekIndex,
		Semester:          resp.Semester,
		SemesterStartDate: resp.SemesterStartDate,
		WeekStartDate:     resp.WeekStartDate,
		WeekEndDate:       resp.WeekEndDate,
		PhaseType:         resp.PhaseType,
		CoursesVisible:    resp.CoursesVisible,
		Events:            events,
	}
}
