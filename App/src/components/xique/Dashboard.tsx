import { useEffect, useState } from "react"
import {
  ArrowRight,
  Clock3,
  MapPin,
  Plus,
  CalendarDays,
  BookOpen,
  Cloud,
  Check,
  ArrowUpRight,
} from "lucide-react"
import type { Course } from "@/types/course"
import type { Page } from "@/App"
import { chinaDate, chinaClock, weekNumber, weekdays } from "./dates"
import { useWeek } from "./use-course-data"
import { EmptyCourses, LoadError, LoadingCards, SemesterPicker } from "./PageParts"
import logo from "@/assets/xique.svg"

interface DashboardProps {
  semester: string
  setSemester: (value: string) => void
  semesters: string[]
  anchor: string
  courses: Course[]
  revision: Date | null
  error: string
  refresh: () => Promise<void>
  navigate: (page: Page) => void
  schoolStatus: string
  openSchool: () => void
  add: () => void
  edit: (course: Course) => void
}
export function Dashboard({
  semester,
  setSemester,
  semesters,
  anchor,
  courses,
  revision,
  error,
  refresh,
  navigate,
  schoolStatus,
  openSchool,
  add,
  edit,
}: DashboardProps) {
  const [clock, setClock] = useState(chinaClock)
  const [date, setDate] = useState(chinaDate)
  useEffect(() => {
    const timer = setInterval(() => {
      setClock(chinaClock())
      setDate(chinaDate())
    }, 30000)
    return () => clearInterval(timer)
  }, [])
  const week = weekNumber(anchor, date),
    weekday = (new Date(`${date}T00:00:00Z`).getUTCDay() + 6) % 7
  const schedule = useWeek(semester, anchor, week, revision, courses)
  const items = (schedule.data?.items ?? []).filter((item) => item.kind === "course")
  const today = items
    .filter((item) => item.day_of_week === weekday + 1)
    .sort((a, b) => a.start_time.localeCompare(b.start_time))
  const next = today.find((item) => item.end_time > clock)
  const ongoing = !!next && next.start_time <= clock
  const fullDate = new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "long",
    day: "numeric",
    weekday: "long",
  }).format(new Date())
  return (
    <>
      <div className="xq-page-heading">
        <div>
          <p className="xq-eyebrow">YOUR DAY, AT A GLANCE</p>
          <h1>
            今天，也从容一点<span className="xq-title-dot">。</span>
          </h1>
          <p className="xq-subtitle">
            {fullDate}
            <span className="xq-text-divider">/</span>把一天的安排，放在心里有数的地方。
          </p>
        </div>
        <SemesterPicker value={semester} values={semesters} onChange={setSemester} />
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
      <section className="xq-hero" aria-label="下一节课程">
        <div className="xq-hero-copy">
          <div className="xq-hero-kicker">
            <span className="xq-live-dot" />
            {next ? (ongoing ? "正在上课" : "下一节课程") : "留一点时间，给自己"}
          </div>
          <h2>
            {schedule.loading
              ? "正在看看今天的安排…"
              : error || schedule.error
                ? "课表暂时无法读取"
                : next?.title || (today.length ? "今天的课程已结束" : "今天，按自己的节奏来")}
          </h2>
          {next ? (
            <div className="xq-hero-details">
              <span>
                <Clock3 size={17} />
                {next.start_time} – {next.end_time}
              </span>
              <span>
                <MapPin size={17} />
                {next.location || "地点待填写"}
              </span>
            </div>
          ) : (
            <p>
              {courses.length
                ? "看看这一周的课表，为下一次出发做好准备。"
                : "先添加你的课程，河小盒会帮你把安排整理好。"}
            </p>
          )}
          <button className="xq-button hero-button" onClick={() => navigate("courses")}>
            查看我的课表
            <ArrowRight size={16} />
          </button>
        </div>
        <div className="xq-hero-art" aria-hidden="true">
          <span className="xq-orbit orbit-one" />
          <span className="xq-orbit orbit-two" />
          <div className="xq-bird-tile">
            <img src={logo} alt="" />
          </div>
          <span className="xq-art-note">
            <Check size={14} />
            安排妥当，轻装出发
          </span>
          <span className="xq-art-spark spark-one">✧</span>
          <span className="xq-art-spark spark-two">✧</span>
        </div>
      </section>
      <div className="xq-stat-grid">
        {[
          {
            label: "今日课程",
            value: today.length,
            suffix: "节",
            icon: CalendarDays,
            note: today.length ? `${weekdays[weekday]}的上课安排` : "享受一点自由时间",
            color: "blue",
          },
          {
            label: "本周安排",
            value: items.length,
            suffix: "节",
            icon: BookOpen,
            note: `第 ${week} 教学周`,
            color: "violet",
          },
          {
            label: "已保存课程",
            value: courses.length,
            suffix: "门次",
            icon: Check,
            note: "当前学期的全部安排",
            color: "green",
          },
        ].map(({ label, value, suffix, icon: Icon, note, color }) => (
          <div className="xq-stat" key={label}>
            <div>
              <span>{label}</span>
              <p>
                {schedule.loading || error || schedule.error ? "—" : value}
                <small>{suffix}</small>
              </p>
              <small>{note}</small>
            </div>
            <span className={`xq-stat-icon ${color}`}>
              <Icon size={21} />
            </span>
          </div>
        ))}
      </div>
      <div className="xq-dashboard-grid">
        <section className="xq-panel">
          <div className="xq-section-heading">
            <div>
              <h2>
                今日安排 <span className="xq-count">{today.length}</span>
              </h2>
              <p>一节一节，踏实向前。</p>
            </div>
            <button className="xq-text-button" onClick={add}>
              <Plus size={16} />
              添加课程
            </button>
          </div>
          {schedule.loading ? (
            <LoadingCards />
          ) : !courses.length ? (
            <EmptyCourses add={add} />
          ) : today.length ? (
            <div className="xq-agenda">
              {today.map((item) => (
                <button
                  className={`xq-agenda-item ${item.end_time <= clock ? "completed" : ""}`}
                  key={item.id}
                  onClick={() => {
                    const course = courses.find((value) => value.id === item.id)
                    if (course) edit(course)
                  }}
                >
                  <div className="xq-agenda-time">
                    <strong>{item.start_time}</strong>
                    <span>{item.end_time}</span>
                  </div>
                  <span className="xq-agenda-line" style={{ backgroundColor: item.color }} />
                  <div className="xq-agenda-body">
                    <strong>{item.title}</strong>
                    <span>
                      <MapPin size={13} />
                      {item.location || "地点待填写"}
                      {item.teacher && <em>· {item.teacher}</em>}
                    </span>
                  </div>
                  <span
                    className={`xq-course-state ${item.end_time <= clock ? "done" : item.start_time <= clock ? "now" : ""}`}
                  >
                    {item.end_time <= clock
                      ? "已结束"
                      : item.start_time <= clock
                        ? "上课中"
                        : "待上课"}
                  </span>
                </button>
              ))}
            </div>
          ) : (
            <div className="xq-day-off">
              <span>☀</span>
              <h3>今天没有课程安排</h3>
              <p>读几页书，散散步，也是一份好安排。</p>
              <button className="xq-text-button" onClick={() => navigate("courses")}>
                看看本周课表
                <ArrowRight size={14} />
              </button>
            </div>
          )}
        </section>
        <aside className="xq-dashboard-aside">
          <section className="xq-panel xq-connect-card">
            <span className="xq-feature-icon">
              <Cloud size={23} />
            </span>
            <h2>学校的安排，及时知道</h2>
            <p>连接河南大学，检查课表变化，核对后再更新你的安排。</p>
            <span className="xq-status-pill neutral">{schoolStatus}</span>
            <div className="xq-connect-foot">
              <button className="xq-text-button" onClick={openSchool} aria-label="查看学校连接">
                连接与检查更新
                <ArrowUpRight size={16} />
              </button>
            </div>
          </section>
          <div className="xq-small-note">
            <span>✦</span>
            <p>
              安排好重要的事，
              <br />
              也给生活留一点空白。
            </p>
          </div>
        </aside>
      </div>
    </>
  )
}
