import { useState } from "react"
import {
  Plus,
  ChevronLeft,
  ChevronRight,
  CalendarDays,
  List,
  MapPin,
  UserRound,
  RefreshCw,
  Clock3,
} from "lucide-react"
import type { Course } from "@/types/course"
import type { WeekScheduleItem } from "@/types/schedule"
import { useWeek } from "./use-course-data"
import { chinaDate, datePlus, weekNumber, weekdays, minutes } from "./dates"
import { EmptyCourses, LoadingCards, LoadError, SemesterPicker } from "./PageParts"
import { HenuImport } from "./HenuImport"
import { CompactWeek } from "@/components/mobile/CompactWeek"
import { formatTeachingWeeks } from "@/lib/mobile/weeks"
import { useTimetableDays } from "@/lib/mobile/timetable-days"
import { ChoicePicker } from "@/components/mobile/ChoicePicker"

interface CoursesPageProps {
  localMode?: boolean
  schoolConnected?: boolean
  localWorkspaceId?: string
  localRevision?: number
  semester: string
  setSemester: (value: string) => void
  semesters: string[]
  anchor: string
  courses: Course[]
  revision: Date | null
  loading: boolean
  error: string
  refresh: () => Promise<void>
  openSchool: () => void
  add: (slot?: { day_of_week: number; start_time: string; end_time: string }) => void
  edit: (course: Course) => void
}
/** Greedy interval lanes keep simultaneous courses individually selectable. */
function courseLanes(items: WeekScheduleItem[]) {
  const sorted = [...items].sort((a, b) => a.start_time.localeCompare(b.start_time))
  const groups: { item: WeekScheduleItem; lane: number; lanes: number }[][] = []
  let end = -1
  for (const item of sorted) {
    if (!groups.length || minutes(item.start_time) >= end) {
      groups.push([])
      end = -1
    }
    const group = groups[groups.length - 1]
    const taken = new Set(
      group
        .filter((value) => minutes(value.item.end_time) > minutes(item.start_time))
        .map((value) => value.lane),
    )
    let lane = 0
    while (taken.has(lane)) lane++
    group.push({ item, lane, lanes: 1 })
    end = Math.max(end, minutes(item.end_time))
  }
  return groups.flatMap((group) => {
    const lanes = Math.max(...group.map((value) => value.lane)) + 1
    return group.map((value) => ({ ...value, lanes }))
  })
}
export function CoursesPage({
  localMode = false,
  schoolConnected = false,
  localWorkspaceId,
  localRevision,
  semester,
  setSemester,
  semesters,
  anchor,
  courses,
  revision,
  loading,
  error,
  refresh,
  openSchool,
  add,
  edit,
}: CoursesPageProps) {
  const timetable = useTimetableDays()
  const visibleWeekdays = localMode ? weekdays.slice(0, timetable.days) : weekdays
  const scope = semester + anchor
  const [importOpen, setImportOpen] = useState(false)
  const [choice, setChoice] = useState({ scope, week: weekNumber(anchor) })
  const limit = localMode ? 18 : 30
  const week = Math.min(limit, choice.scope === scope ? choice.week : weekNumber(anchor))
  const [mode, setMode] = useState("week")
  const [day, setDay] = useState((new Date(`${chinaDate()}T00:00:00Z`).getUTCDay() + 6) % 7)
  const schedule = useWeek(semester, anchor, week, revision, courses)
  const items = (schedule.data?.items ?? []).filter((item) => item.kind === "course")
  const changeWeek = (value: number) => setChoice({ scope, week: Math.max(1, Math.min(limit, value)) })
  const open = (item: WeekScheduleItem) => {
    const course = courses.find((value) => value.id === item.id)
    if (course) edit(course)
  }
  const firstHour = Math.min(7, ...items.map((item) => Math.floor(minutes(item.start_time) / 60)))
  const lastHour = Math.max(22, ...items.map((item) => Math.ceil(minutes(item.end_time) / 60)))
  const weekStart = schedule.data?.week_start_date ?? chinaDate()
  function courseList(dayIndex?: number) {
    const list = items
      .filter((item) => dayIndex === undefined || item.day_of_week === dayIndex + 1)
      .sort((a, b) => a.day_of_week - b.day_of_week || a.start_time.localeCompare(b.start_time))
    return list.length ? (
      <div className="xq-course-list">
        {list.map((item) => (
          <button key={item.id} className="xq-course-list-item" onClick={() => open(item)}>
            <span className="xq-list-color" style={{ backgroundColor: item.color }} />
            <div className="xq-list-title">
              <strong>{item.title}</strong>
              <span>
                <MapPin size={13} />
                {item.location || "地点待填写"}
                <em>· {item.teacher || "教师待填写"}</em>
              </span>
            </div>
            <div className="xq-list-time">
              <strong>
                {item.start_time} – {item.end_time}
              </strong>
              <span>
                {weekdays[item.day_of_week - 1]} · {formatTeachingWeeks(item.week_pattern)}
              </span>
            </div>
            <ChevronRight size={16} />
          </button>
        ))}
      </div>
    ) : (
      <div className="xq-day-off compact">
        <CalendarDays size={28} />
        <h3>{dayIndex === undefined ? "这一周暂无课程" : "这一天没有课"}</h3>
        <p>试试切换周次，或添加一门新的课程。</p>
      </div>
    )
  }
  return (
    <>
      {importOpen && (
        <HenuImport
          key={semester}
          semester={semester}
          close={() => setImportOpen(false)}
          onCommitted={refresh}
        />
      )}
      <div className="xq-page-heading">
        <div>
          <p className="xq-eyebrow">A LITTLE ORDER, A LOT OF EASE</p>
          <h1>
            我的课表<span className="xq-title-dot">。</span>
          </h1>
          <p className="xq-subtitle">把每一节课，安排得明明白白。</p>
        </div>
        <div className="xq-henu-actions">
          <button className="xq-button" onClick={openSchool}>
            <RefreshCw size={16} />
            学校课表更新
          </button>
          {!localMode && (
            <button className="xq-button" onClick={() => setImportOpen(true)}>
              河大课表预览
            </button>
          )}
          {!localMode && (
            <button className="xq-button primary" onClick={() => add()}>
              <Plus size={17} />
              添加课程
            </button>
          )}
        </div>
      </div>
      <div className="xq-schedule-toolbar">
        <SemesterPicker value={semester} values={semesters} onChange={setSemester} />
        <div className="xq-week-control">
          <button
            className="xq-icon-button"
            disabled={week <= 1}
            onClick={() => changeWeek(week - 1)}
            aria-label="上一周"
          >
            <ChevronLeft size={17} />
          </button>
          {localMode ? <ChoicePicker title="教学周" value={String(week)} weeks options={Array.from({ length: limit }, (_, i) => ({ value: String(i + 1), label: `第 ${i + 1} 周` }))} choose={v => changeWeek(Number(v))} /> : <label>
            <span className="sr-only">教学周</span>
            <select
              aria-label="教学周"
              value={week}
              onChange={(event) => changeWeek(Number(event.target.value))}
            >
              {Array.from({ length: limit }, (_, index) => (
                <option key={index + 1} value={index + 1}>
                  第 {index + 1} 周
                </option>
              ))}
            </select>
          </label>}
          <button
            className="xq-icon-button"
            disabled={week >= limit}
            onClick={() => changeWeek(week + 1)}
            aria-label="下一周"
          >
            <ChevronRight size={17} />
          </button>
          <button className="xq-current-week" onClick={() => changeWeek(weekNumber(anchor))}>
            本周
          </button>
        </div>
        <div className="xq-view-actions">
          {localMode && <button type="button" className="xq-icon-button" aria-label="添加课程" onClick={() => add()}><Plus size={18} /></button>}
          <div className="xq-view-toggle">
            <button
              className={mode === "week" ? "active" : ""}
              onClick={() => setMode("week")}
              aria-label="周视图"
              aria-pressed={mode === "week"}
            >
              <CalendarDays size={16} />
            </button>
            <button
              className={mode === "list" ? "active" : ""}
              onClick={() => setMode("list")}
              aria-label="列表视图"
              aria-pressed={mode === "list"}
            >
              <List size={17} />
            </button>
          </div>
          <button
            className="xq-icon-button"
            aria-label="刷新课表"
            disabled={loading}
            onClick={() => void refresh()}
          >
            <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          </button>
        </div>
      </div>
      {(error || schedule.error) && (
        <LoadError
          error={error || schedule.error}
          retry={() => {
            void refresh()
            schedule.retry()
          }}
        />
      )}
      <section className={`xq-panel xq-schedule-panel ${localMode ? `xm-mode-${mode}` : ""}`}>
        {(!localMode || courses.length > 0) && <div className="xq-schedule-caption">
          <span>
            <span className="xq-live-dot" />
            {weekStart.replaceAll("-", ".")} — {datePlus(weekStart, 6).slice(5).replace("-", ".")}
          </span>
          <span>
            {items.length} 节课程 · 点击课程{localMode ? "查看详情" : "可编辑"}
          </span>
        </div>}
        {loading || schedule.loading ? (
          <LoadingCards />
        ) : !courses.length ? (
          localMode ? <div className="xm-login-timetable">
            <div className="xm-login-mark"><CalendarDays size={28} /></div>
            <h3>把整学期课表带在身边</h3>
            <p>{schoolConnected ? "学校账号已连接，选择校区即可导入课表。" : "登录学校账号，选择校区，课表自动保存到手机。"}</p>
            <button className="xm-primary" onClick={openSchool}>{schoolConnected ? "导入学校课表" : "登录并导入课表"}</button>
            <small>保存后，无网也能查看</small>
          </div> : <EmptyCourses add={() => add()} />
        ) : (
          <>
            <div className="xq-day-tabs" aria-label="选择星期">
              {weekdays.map((label, index) => (
                <button
                  key={label}
                  className={day === index ? "active" : ""}
                  aria-pressed={day === index}
                  onClick={() => setDay(index)}
                >
                  <span>{label}</span>
                  <strong>{datePlus(weekStart, index).slice(8)}</strong>
                  <i
                    className={
                      items.some((item) => item.day_of_week === index + 1) ? "has-course" : ""
                    }
                  />
                </button>
              ))}
            </div>
            <div className="xq-mobile-courses">
              {localMode ? (mode === "list" ? courseList() : null) : courseList(day)}
            </div>
            <div className="xq-desktop-courses">
              {mode === "list" ? (
                courseList()
              ) : localMode ? (
                <CompactWeek
                  key={`${semester}:${weekStart}:${visibleWeekdays.length}`}
                  workspaceId={localWorkspaceId}
                  semester={semester}
                  revision={localRevision}
                  days={visibleWeekdays.map((_, index) =>
                    courseLanes(items.filter((item) => item.day_of_week === index + 1)),
                  )}
                  weekStart={weekStart}
                  open={open}
                  add={add}
                />
              ) : (
                <div className="xq-grid-scroll">
                  <div className="xq-grid-headers">
                    <div>
                      <Clock3 size={16} />
                    </div>
                    {weekdays.map((label, index) => (
                      <div
                        key={label}
                        className={datePlus(weekStart, index) === chinaDate() ? "today" : ""}
                      >
                        <span>{label}</span>
                        <strong>{datePlus(weekStart, index).slice(5).replace("-", "/")}</strong>
                      </div>
                    ))}
                  </div>
                  <div className="xq-week-grid" style={{ height: (lastHour - firstHour) * 54 }}>
                    <div className="xq-time-axis">
                      {Array.from({ length: lastHour - firstHour }, (_, index) => (
                        <span key={index} style={{ top: index * 54 }}>
                          {String(firstHour + index).padStart(2, "0")}:00
                        </span>
                      ))}
                    </div>
                    {weekdays.map((label, index) => (
                      <div
                        className={`xq-day-column ${datePlus(weekStart, index) === chinaDate() ? "today" : ""}`}
                        key={label}
                      >
                        {courseLanes(items.filter((item) => item.day_of_week === index + 1)).map(
                          ({ item, lane, lanes }) => (
                            <button
                              key={item.id}
                              className="xq-course-block"
                              style={{
                                top: (minutes(item.start_time) - firstHour * 60) * 0.9 + 2,
                                height: Math.max(
                                  28,
                                  (minutes(item.end_time) - minutes(item.start_time)) * 0.9 - 4,
                                ),
                                left: `calc(${(lane / lanes) * 100}% + 3px)`,
                                width: `calc(${100 / lanes}% - 6px)`,
                                backgroundColor: item.color,
                              }}
                              onClick={() => open(item)}
                              title={`${item.title} ${item.start_time}-${item.end_time} ${item.location}`}
                            >
                              <strong>{item.title}</strong>
                              <span>
                                {item.start_time}–{item.end_time}
                              </span>
                              <span>
                                <MapPin size={11} />
                                {item.location || "待填写"}
                              </span>
                              {minutes(item.end_time) - minutes(item.start_time) >= 80 && (
                                <span>
                                  <UserRound size={11} />
                                  {item.teacher || "待填写"}
                                </span>
                              )}
                            </button>
                          ),
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </>
        )}
      </section>
      {(!localMode || courses.length > 0) && <div className="xq-schedule-note">
        <span>
          <i />
          {localMode ? "点击课程编辑 · 空白格点两次加课" : "课程根据上课周次显示"}
        </span>
        <span>{localMode ? timetable.days === 5 ? "五天显示 · 列表可看周末" : "整周显示 · 可切换列表" : `第一教学周所在日期：${anchor}`}</span>
      </div>}
    </>
  )
}
