import { useState, type FormEvent } from "react"
import { Save, Trash2, MapPin, CalendarDays } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import { createCourse, updateCourse, deleteCourse } from "@/lib/api/courses"
import { userErrorMessage } from "@/lib/errors"
import type { Course } from "@/types/course"
import type { CourseDefaults } from "@/App"
import { weekdays } from "./dates"
import { localWorkspaceBridge, localSchoolError } from "@/lib/mobile/local-school"
import { formatTeachingWeeks } from "@/lib/mobile/weeks"
import { COURSE_COLORS } from "@/components/mobile/course-colors"

const colors = [
  { value: "#C9DCF9", name: "晴蓝" },
  { value: "#D9D2F4", name: "淡紫" },
  { value: "#C4E6D9", name: "薄荷" },
  { value: "#F6DEC1", name: "暖杏" },
  { value: "#F1CAD7", name: "浅粉" },
  { value: "#CCE7EF", name: "湖青" },
]
export function CourseEditor({
  course,
  defaults,
  close,
  saved,
  local,
  slot,
}: {
  course: Course | null
  defaults: CourseDefaults
  close: () => void
  saved: (message: string) => Promise<void>
  local?: { workspaceId: string; revision: number }
  slot?: { day_of_week: number; start_time: string; end_time: string }
}) {
  const editorColors = local ? COURSE_COLORS.map(c => ({ value: c.value, name: c.label })) : colors
  const [form, setForm] = useState({
    name: course?.name ?? "",
    day_of_week: course?.day_of_week ?? slot?.day_of_week ?? 1,
    start_time: course?.start_time ?? slot?.start_time ?? "08:30",
    end_time: course?.end_time ?? slot?.end_time ?? "09:15",
    week_pattern: course?.week_pattern ? (local ? formatTeachingWeeks(course.week_pattern) : course.week_pattern) : (local ? "1-18周" : "1-16周"),
    semester_start_date: course?.semester_start_date || defaults.anchor,
    location: course?.location ?? "",
    teacher: course?.teacher ?? "",
    color: course?.color ?? editorColors[0].value,
    semester: course?.semester || defaults.semester,
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [syncId] = useState(() => course?.sync_id ?? crypto.randomUUID())
  const change = (key: keyof typeof form, value: string | number) =>
    setForm((previous) => ({ ...previous, [key]: value }))
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy) return
    if (form.start_time >= form.end_time) {
      setError("结束时间需要晚于开始时间")
      return
    }
    const segments = form.week_pattern.replace(/\s/g, "").split(/[,，;；、/]/)
    if (
      segments.some((segment) => {
        const match = /^第?([1-9]\d?)(?:-([1-9]\d?))?(?:周(?:[（(]?[单双全]周?[)）]?)?)?$/.exec(
          segment,
        )
        return !match || (match[2] && Number(match[2]) < Number(match[1])) || (local && Number(match[2] ?? match[1]) > 18)
      })
    ) {
      setError("请检查周次，例如 1-16周、1-16周(单) 或 1-4周,6-8周")
      return
    }
    setBusy(true)
    setError("")
    try {
      const body = {
        ...form,
        name: form.name.trim(),
        semester: form.semester.trim(),
        week_pattern: form.week_pattern.replace(/\s/g, ""),
      }
      if (local) await localWorkspaceBridge.saveCourse({ workspace_id: local.workspaceId, expected_revision: local.revision, course_id: course?.id ?? 0, sync_id: syncId, course: body })
      else if (course) await updateCourse(course.id, body)
      else await createCourse(body)
      await saved(course ? "课程已更新" : "新课程已保存")
    } catch (err) {
      setError(local ? localSchoolError(err) : userErrorMessage(err, "保存失败，请重试"))
    } finally {
      setBusy(false)
    }
  }
  async function remove() {
    if (!course || busy) return
    setBusy(true)
    setError("")
    try {
      if (local) await localWorkspaceBridge.deleteCourse({ workspace_id: local.workspaceId, expected_revision: local.revision, course_id: course.id })
      else await deleteCourse(course.id)
      await saved("课程已删除")
    } catch (err) {
      setError(local ? localSchoolError(err) : userErrorMessage(err, "删除失败，请重试"))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open
      onOpenChange={(value) => {
        if (!value && !busy) close()
      }}
      title={course ? "编辑课程" : "添加一门新课程"}
      description={local ? "修改仅保存在手机，学校原始课表仍保留。" : "填好时间与地点，把新的安排放进课表。"}
      className="xq-course-modal"
    >
      <form className="xq-course-form" onSubmit={submit}>
        <fieldset disabled={busy}>
          <div className="xq-field">
            <label htmlFor="xq-course-name">
              课程名称 <span>*</span>
            </label>
            <input
              id="xq-course-name"
              autoFocus
              required
              maxLength={120}
              placeholder="例如：高等数学"
              value={form.name}
              onChange={(event) => change("name", event.target.value)}
            />
          </div>
          <div className="xq-form-row">
            <div className="xq-field">
              <label htmlFor="xq-course-day">上课日期</label>
              <select
                id="xq-course-day"
                value={form.day_of_week}
                onChange={(event) => change("day_of_week", Number(event.target.value))}
              >
                {weekdays.map((day, index) => (
                  <option key={day} value={index + 1}>
                    {day}
                  </option>
                ))}
              </select>
            </div>
            <div className="xq-field">
              <label htmlFor="xq-course-teacher">授课教师</label>
              <input
                id="xq-course-teacher"
                value={form.teacher}
                onChange={(event) => change("teacher", event.target.value)}
                placeholder="选填"
                maxLength={120}
              />
            </div>
          </div>
          <div className="xq-form-row">
            <div className="xq-field">
              <label htmlFor="xq-course-start">开始时间</label>
              <input
                id="xq-course-start"
                type="time"
                required
                value={form.start_time}
                onChange={(event) => change("start_time", event.target.value)}
              />
            </div>
            <div className="xq-field">
              <label htmlFor="xq-course-end">结束时间</label>
              <input
                id="xq-course-end"
                type="time"
                required
                value={form.end_time}
                onChange={(event) => change("end_time", event.target.value)}
              />
            </div>
          </div>
          <div className="xq-field">
            <label htmlFor="xq-course-room">
              <MapPin size={14} />
              上课地点
            </label>
            <input
              id="xq-course-room"
              value={form.location}
              onChange={(event) => change("location", event.target.value)}
              placeholder="例如：综合教学楼 A201"
              maxLength={160}
            />
          </div>
          <div className="xq-form-row">
            <div className="xq-field">
              <label htmlFor="xq-course-weeks">上课周次</label>
              <input
                id="xq-course-weeks"
                required
                value={form.week_pattern}
                onChange={(event) => change("week_pattern", event.target.value)}
                aria-describedby="xq-weeks-help"
                placeholder="1-16周"
              />
            </div>
            <div className="xq-field">
              <label htmlFor="xq-course-anchor">
                <CalendarDays size={14} />
                第一教学周所在日期
              </label>
              <input
                id="xq-course-anchor"
                readOnly={!!local}
                type="date"
                required
                value={form.semester_start_date}
                onChange={(event) => change("semester_start_date", event.target.value)}
              />
            </div>
          </div>
          <p id="xq-weeks-help" className="xq-field-hint">
            单双周可写为 1-16周(单) 或 2-16周(双)。
          </p>
          <div className="xq-form-row">
            <div className="xq-field">
              <label htmlFor="xq-course-semester">所属学期</label>
              <input
                id="xq-course-semester"
                readOnly={!!local}
                required
                maxLength={60}
                value={form.semester}
                onChange={(event) => change("semester", event.target.value)}
                placeholder="例如：2026S2"
              />
            </div>
            <div className="xq-field">
              <label>课程颜色</label>
              <div className="xq-color-options">
                {editorColors.map((color) => (
                  <button
                    type="button"
                    key={color.value}
                    aria-label={`${color.name}课程颜色`}
                    aria-pressed={form.color === color.value}
                    className={form.color === color.value ? "selected" : ""}
                    style={{ backgroundColor: color.value }}
                    onClick={() => change("color", color.value)}
                  />
                ))}
              </div>
            </div>
          </div>
        </fieldset>
        {error && (
          <p className="xq-form-error" role="alert">
            {error}
          </p>
        )}
        {confirmDelete ? (
          <div className="xq-delete-confirm">
            <p>确定删除“{course?.name}”吗？</p>
            <div>
              <button
                type="button"
                className="xq-button small"
                disabled={busy}
                onClick={() => setConfirmDelete(false)}
              >
                保留课程
              </button>
              <button
                type="button"
                className="xq-button danger small"
                disabled={busy}
                onClick={() => void remove()}
              >
                确定删除
              </button>
            </div>
          </div>
        ) : (
          <div className="xq-form-footer">
            {course && (
              <button
                className="xq-text-button destructive"
                type="button"
                disabled={busy}
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 size={15} />
                删除课程
              </button>
            )}
            <div>
              <button type="button" className="xq-button" disabled={busy} onClick={close}>
                取消
              </button>
              <button
                className="xq-button primary"
                type="submit"
                disabled={busy || !form.name.trim() || !form.semester.trim()}
              >
                <Save size={16} />
                {busy ? "正在保存…" : "保存课程"}
              </button>
            </div>
          </div>
        )}
      </form>
    </Modal>
  )
}
