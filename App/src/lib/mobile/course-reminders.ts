import { Capacitor, registerPlugin } from "@capacitor/core"
export interface CourseReminderState { enabled: boolean; minutes: number; notification_permission: boolean; exact_alarm: boolean; next_at: number; schedule_error: boolean; open_timetable?: boolean }
interface Bridge {
  getState(): Promise<CourseReminderState>
  configure(input: { enabled: boolean; minutes: number }): Promise<CourseReminderState>
  openExactSettings(): Promise<void>
  openNotificationSettings(): Promise<void>
}
export const courseRemindersAvailable = () => Capacitor.isPluginAvailable("XiqueCourseReminders")
export const courseReminders = registerPlugin<Bridge>("XiqueCourseReminders")
