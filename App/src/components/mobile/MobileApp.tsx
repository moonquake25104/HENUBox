import { useCallback, useEffect, useRef, useState } from "react"
import {
  ArrowRight,
  CalendarDays,
  ChevronRight,
  Clock3,
  ImagePlus,
  MapPin,
  Smartphone,
  Timer,
  UserRound,
} from "lucide-react"
import { LocalSchoolPanel } from "@/components/xique/LocalSchoolPanel"
import { CoursesPage } from "@/components/xique/CoursesPage"
import { CourseEditor } from "@/components/xique/CourseEditor"
import { formatTeachingWeeks } from "@/lib/mobile/weeks"
import { Modal } from "@/components/shared/modal"
import {
  chinaClock,
  chinaDate,
  currentSemester,
  weekNumber,
} from "@/components/xique/dates"
import {
  localWorkspaceBridge,
  localSchoolBridge,
  localSchoolError,
  type LocalBootstrap,
  type LocalConnection,
} from "@/lib/mobile/local-school"
import { localWeek, validSnapshot } from "@/lib/mobile/offline-timetable"
import { useMobilePresentation } from "@/lib/mobile/presentation"
import { MeSettings } from "./MeSettings"
import { syncSystemAppearance } from "@/lib/mobile/appearance"
import { useAndroidBackNavigation } from "@/lib/mobile/back"
import { libraryBrowser } from "@/lib/mobile/library"
import { emailBrowser } from "@/lib/mobile/email"
import type { Course } from "@/types/course"
import { CampusArt } from "./CampusArt"
import { CampusThemeArt } from "./CampusThemeArt"
import { courseReminders, courseRemindersAvailable } from "@/lib/mobile/course-reminders"
import { ClassroomPage } from "./ClassroomLivePage"
import { COURSE_COLORS, courseDisplayColor } from "./course-colors"
import { HomeBackgroundSettings } from "./HomeBackground"
import { useHomeBackground } from "@/hooks/use-home-background"
import { useFocus } from "@/hooks/use-focus"
import { focusTime } from "@/lib/mobile/focus"
import { FocusPage } from "./FocusPage"
import { MobileNavigation } from "./MobileNavigation"
import { mobileNavigation as navigation, type MobilePage } from "@/lib/mobile/navigation"
import { useUiStyle } from "@/lib/mobile/ui-style"
import { useContentTransition } from "@/hooks/use-content-transition"
import { useModalPresence } from "@/lib/mobile/modal-presence"
import { useIntroNotice } from "@/hooks/use-intro-notice"
import { FirstRunNotice } from "./FirstRunNotice"
import defaultHomeCard from "@/assets/home-card-default.svg"
import { useHomeMotion } from "@/hooks/use-home-motion"
import { HomeVideo } from "./HomeVideo"
import { useTodos } from "@/hooks/use-todos"
import { HomeTodos, TodoPage } from "./TodoPage"
import { CampusGuidePage } from "./CampusGuidePage"
import "./mobile-v2.css"
import "./mobile-compact.css"
import "./liquid-glass.css"
import "./todos.css"
import "./android-native.css"

type Feature = "electricity" | "classroom" | "library"
const features = {
  electricity: {
    name: "电费查询",
    detail: "剩余电量 · 每日用量",
    subtitle: "用电情况，心里有数。",
    description: "这里将显示本人电表的剩余电量、抄表时间与每日用量，离线时也能查看上次保存的结果。",
    status: "功能已预留，手机授权与接入暂未开放。",
  },
  classroom: {
    name: "空闲教室",
    detail: "找一间，安静自习",
    subtitle: "给专注，找个好位置。",
    description:
      "这里将支持按日期、节次和教学楼查找没有课程安排的教室。实际开放情况仍需结合学校信息确认。",
    status: "功能已预留，学校查询接口待验证。",
  },
  library: {
    name: "图书馆预约",
    detail: "留一段，属于你的时间",
    subtitle: "把阅读，放进日常。",
    description: "在 App 内打开河南大学图书馆预约官网，由你直接在网页登录和预约。",
    status: "官方预约网页",
  },
} as const
function initialPage(): MobilePage {
  const value = location.hash.slice(2)
  return navigation.some((item) => item.id === value) ? (value as MobilePage) : "home"
}
function savedSemester() {
  try {
    return localStorage.getItem("xique-local-semester") || currentSemester()
  } catch {
    return currentSemester()
  }
}
function isBootstrap(value: LocalBootstrap): boolean {
  return (
    value?.schema_version === 2 &&
    value.mode === "LOCAL" &&
    /^[0-9a-f-]{36}$/.test(value.workspace_id) &&
    Number.isSafeInteger(value.revision) &&
    value.revision >= 0 &&
    validSnapshot({
      schema: 1,
      saved_at: value.saved_at ?? "1970-01-01T00:00:00Z",
      courses: value.courses,
    })
  )
}
function savedTime(value: string | null) {
  return value
    ? new Date(value).toLocaleString("zh-CN", {
        timeZone: "Asia/Shanghai",
        month: "numeric",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "还没有保存"
}

export function MobileApp() {
  const notice = useIntroNotice(), modalOpen = useModalPresence()
  const ui = useUiStyle()
  useEffect(() => {
    if (courseRemindersAvailable()) void courseReminders.getState().then(s => { if (s.open_timetable) location.hash = "#/timetable" }).catch(() => {})
  }, [])
  const focus = useFocus()
  const todos = useTodos()
  const [todoOpen, setTodoOpen] = useState(false)
  const [guideOpen, setGuideOpen] = useState(false)
  const [todoCreate, setTodoCreate] = useState(0)
  const openTodos = (create = false) => { setGuideOpen(false); setFocusOpen(false); setSchoolOpen(false); setClassroomOpen(false); setFeature(null); setCourse(null); setEditor(null); setBackgroundOpen(false); setTodoCreate(create ? Date.now() : 0); setTodoOpen(true) }
  const [focusOpen, setFocusOpen] = useState(false)
  useEffect(() => { if (focus.state?.open_requested) queueMicrotask(() => { setGuideOpen(false); setTodoOpen(false); setTodoCreate(0); setFocusOpen(true) }) }, [focus.state])
  const background = useHomeBackground()
  const [backgroundOpen, setBackgroundOpen] = useState(false)
  const [boot, setBoot] = useState<LocalBootstrap | null>(null)
  const [connection, setConnection] = useState<LocalConnection | null>(null)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState<MobilePage>(initialPage)
  const [semester, setSemester] = useState(savedSemester)
  const [schoolOpen, setSchoolOpen] = useState(false)
  const [classroomOpen, setClassroomOpen] = useState(false)
  const resumeClassroom = useRef(false)
  const [feature, setFeature] = useState<Feature | null>(null)
  const [libraryOpening, setLibraryOpening] = useState(false)
  const [libraryError, setLibraryError] = useState("")
  const [emailOpening, setEmailOpening] = useState(false)
  const [emailError, setEmailError] = useState("")
  const [course, setCourse] = useState<Course | null>(null)
  const [editor, setEditor] = useState<{ course: Course | null; slot?: { day_of_week: number; start_time: string; end_time: string } } | null>(null)
  const [colorBusy, setColorBusy] = useState(false)
  const [colorError, setColorError] = useState("")
  const [clock, setClock] = useState(chinaClock)
  const [date, setDate] = useState(chinaDate)
  const motion = useHomeMotion(page === "home" && !modalOpen && !notice.open && !backgroundOpen && !schoolOpen && !focusOpen && !todoOpen && !guideOpen && !course && !editor)
  const presentation = useMobilePresentation(ui.style)
  const content = useRef<HTMLElement>(null)
  useContentTransition(content, page, navigation.findIndex(n => n.id === page), !!boot && !(modalOpen || notice.open || guideOpen || focusOpen || todoOpen || schoolOpen || classroomOpen || course || editor || backgroundOpen || feature))
  const { theme } = presentation
  const alive = useRef(true)
  const request = useRef(0)
  useAndroidBackNavigation(() => {
    if (page === "home") return false
    navigate("home")
    return true
  })
  useEffect(() => {
    void syncSystemAppearance(theme, ui.style, presentation.palette, presentation.mode)
  }, [theme, ui.style, presentation.palette, presentation.mode])
  const refresh = useCallback(async () => {
    const current = ++request.current
    try {
      const value = await localWorkspaceBridge.bootstrap()
      if (!isBootstrap(value)) throw new Error("Invalid local timetable")
      if (!alive.current || request.current !== current) return
      setBoot(value)
      setError("")
      try {
        const status = await localSchoolBridge.getConnection({ workspace_id: value.workspace_id })
        if (alive.current && request.current === current) setConnection(status)
      } catch {
        /* Local courses remain readable when school state is unavailable. */
      }
    } catch (err) {
      if (alive.current && request.current === current) setError(localSchoolError(err))
      throw err
    } finally {
      if (alive.current && request.current === current) setLoading(false)
    }
  }, [])
  useEffect(() => {
    let live = true
    alive.current = true
    document.body.classList.add("xique-mobile-v2")
    queueMicrotask(() => {
      if (live) void refresh().catch(() => {})
    })
    const changed = () => { setGuideOpen(false); setPage(initialPage()); document.getElementById("xm-content")?.scrollTo({ top: 0 }) }
    const timer = window.setInterval(() => {
      setClock(chinaClock())
      setDate(chinaDate())
    }, 30000)
    window.addEventListener("hashchange", changed)
    return () => {
      live = false
      alive.current = false
      document.body.classList.remove("xique-mobile-v2")
      window.clearInterval(timer)
      window.removeEventListener("hashchange", changed)
    }
  }, [refresh])
  function navigate(next: MobilePage) {
    setGuideOpen(false)
    location.hash = `/${next}`
    setPage(next)
    document.getElementById("xm-content")?.scrollTo({ top: 0 })
    window.scrollTo({ top: 0 })
  }
  function chooseSemester(value: string) {
    setSemester(value)
    try {
      localStorage.setItem("xique-local-semester", value)
    } catch {
      /* selection still works */
    }
  }
  const allCourses = boot?.courses ?? []
  const semesters = [...new Set([semester, ...allCourses.map((c) => c.semester)])]
  const courses = allCourses
    .filter((c) => c.semester === semester)
    .map((c) => ({ ...c, color: courseDisplayColor(c) }))
  const anchor = courses[0]?.semester_start_date
  const week = anchor ? weekNumber(anchor, date) : null
  const today =
    anchor && week
      ? localWeek(courses, semester, anchor, week).items.filter(
          (c) => c.day_of_week === ((new Date(`${date}T00:00:00Z`).getUTCDay() + 6) % 7) + 1,
        )
      : []
  const next = today.find((item) => item.end_time > clock)
  const openSchool = () => setSchoolOpen(true)
  async function openLibrary() {
    if (libraryOpening) return
    setLibraryOpening(true)
    setLibraryError("")
    try {
      await libraryBrowser.open()
    } catch {
      setLibraryError("暂时无法打开图书馆网页，请重试。")
    } finally {
      setLibraryOpening(false)
    }
  }
  async function openEmail() {
    if (emailOpening) return
    setEmailOpening(true); setEmailError("")
    try { await emailBrowser.open() }
    catch { setEmailError("暂时无法打开邮箱开户网页，请重试。") }
    finally { setEmailOpening(false) }
  }
  async function changeCourseColor(color: string) {
    if (!boot || !course || colorBusy) return
    setColorBusy(true)
    setColorError("")
    try {
      const value = await localWorkspaceBridge.setCourseColor({
        workspace_id: boot.workspace_id,
        course_id: course.id,
        color,
      })
      setBoot((current) =>
        current
          ? {
              ...current,
              revision: value.revision,
              courses: current.courses.map((item) =>
                item.id === course.id ? { ...item, color: value.color } : item,
              ),
            }
          : current,
      )
      setCourse((current) => (current ? { ...current, color: value.color } : current))
    } catch (error) {
      setColorError(localSchoolError(error))
    } finally {
      setColorBusy(false)
    }
  }
  if (!boot)
    return (
      <div className="xm-app xm-boot">
        <Smartphone size={34} />
        <h1>HENUBox</h1>
        <p>{loading ? "正在打开本机课表…" : "本机课表暂未能读取，原数据仍保留。"}</p>
        {error && <p role="alert">{error}</p>}
        {!loading && (
          <button
            className="xm-primary"
            onClick={() => {
              setLoading(true)
              void refresh().catch(() => {})
            }}
          >
            重新读取
          </button>
        )}
        <FirstRunNotice notice={notice} />
      </div>
    )

  return (
    <div className={`xm-app xm-page-${page}`}>
      <a href="#xm-content" className="xq-skip">
        跳到主要内容
      </a>
      {page === "campus" && (
        <header className="xm-heading" inert={focusOpen || todoOpen} aria-hidden={focusOpen || todoOpen || undefined}>
          <div>
            <h1>Campus</h1>
            <p>让校园生活，简单一点。</p>
          </div>
        </header>
      )}
      <main ref={content} id="xm-content" inert={focusOpen || todoOpen} aria-hidden={focusOpen || todoOpen || undefined} aria-label={navigation.find((item) => item.id === page)?.title}>
        {error && (
          <div className="xm-notice" role="alert">
            {error}
            <button onClick={() => void refresh().catch(() => {})}>重试</button>
          </div>
        )}
        {boot.migration_warning && (
          <p className="xm-notice" role="status">
            {boot.migration_warning}
          </p>
        )}
        {page === "home" && (
          <>
            <button className="xm-daily-note" onClick={() => navigate("timetable")}>
              <span>
                {week ? `第 ${week} 教学周` : "把时间，留给重要的事。"}
                <small>
                  {date.replaceAll("-", ".")}{" "}
                  {week ? "· 今天也要好好生活" : "课表保存在手机，随时打开看看"}
                </small>
              </span>
              <CalendarDays size={22} />
            </button>
            <div className="xm-hero-wrap"><button
              className="xm-hero has-photo"
              onClick={
                next
                  ? () => setCourse(courses.find((c) => c.id === next.id) ?? null)
                  : allCourses.length
                    ? () => navigate("timetable")
                    : openSchool
              }
            >
              {background.src ? background.video ? <HomeVideo src={background.src} poster={background.poster} playing={motion.playing} /> : <img className="xm-hero-photo" data-animated={background.animated && motion.playing || undefined} src={background.animated && !motion.playing ? background.poster ?? background.src : background.src} alt="" /> : <img className="xm-hero-photo" src={defaultHomeCard} alt="" />}
              <div className="xm-hero-copy">
                <span className="xm-light-label">
                  {next
                    ? next.start_time <= clock
                      ? "正在上课"
                      : "下一节课程"
                    : allCourses.length
                      ? "今日安排"
                      : "从一张课表开始"}
                </span>
                <h2>
                  {next?.title ??
                    (allCourses.length
                      ? today.length
                        ? "今天的课程已结束。"
                        : "今天，给自己留点空闲。"
                      : "把整学期，装进口袋。")}
                </h2>
                <p>
                  {next
                    ? `${next.start_time}–${next.end_time} · ${next.location || "教室待确认"}`
                    : allCourses.length
                      ? "打开课表，看看接下来的安排。"
                      : "连接河南大学，核对后保存到手机。"}
                </p>
                <span className="xm-hero-link">
                  {next ? "查看课程" : allCourses.length ? "查看课表" : "连接学校"}
                  <ArrowRight size={16} />
                </span>
              </div>
            </button><div className="xm-hero-tools"><button type="button" className="xm-hero-customize" aria-label="更换首页背景" onClick={() => setBackgroundOpen(true)}><ImagePlus size={16} /><span>换背景</span></button></div></div>
            {focus.state?.timer && <button className="xm-focus-strip" onClick={() => setFocusOpen(true)}><Timer size={21} /><span>{focus.state.timer.status === "RUNNING" ? "专注进行中" : "计时已暂停"}<small>{focus.state.timer.label || "点击返回专注时光"}</small></span><strong>{focusTime(focus.state.timer.mode === "countup" ? focus.state.timer.elapsed_ms : focus.state.timer.remaining_ms)}</strong><ChevronRight size={17} /></button>}
            <section className="xm-today">
              <div className="xm-section-title">
                <h2>今天的课程</h2>
                <button onClick={() => navigate("timetable")}>
                  全部
                  <ArrowRight size={16} />
                </button>
              </div>
              {today.length ? (
                today.map((item) => (
                  <button
                    className={`xm-day-row ${item.end_time <= clock ? "finished" : ""}`}
                    key={item.id}
                    onClick={() => setCourse(courses.find((c) => c.id === item.id) ?? null)}
                  >
                    <time>
                      {item.start_time}
                      <small>{item.end_time}</small>
                    </time>
                    <span>
                      <strong>{item.title}</strong>
                      <small>
                        <MapPin size={12} />
                        {item.location || "教室待确认"}
                      </small>
                    </span>
                    <ChevronRight size={16} />
                  </button>
                ))
              ) : (
                <p className="xm-empty-note">
                  {allCourses.length
                    ? "今天没有课程安排，做点喜欢的事吧。"
                    : "还没有本机课表，连接学校后即可导入。"}
                </p>
              )}
            </section>
            <HomeTodos todos={todos} open={() => openTodos()} add={() => openTodos(true)} />
          </>
        )}
        {page === "timetable" && (
          <div className="xm-timetable">
            <CoursesPage
              schoolConnected={connection?.connection_state === "CONNECTED"}
              localWorkspaceId={boot.workspace_id}
              localRevision={boot.revision}
              localMode
              semester={semester}
              setSemester={chooseSemester}
              semesters={semesters}
              anchor={anchor ?? date}
              courses={courses}
              revision={boot.saved_at ? new Date(boot.saved_at) : null}
              loading={false}
              error=""
              refresh={async () => {
                await refresh().catch(() => {})
              }}
              openSchool={openSchool}
              add={(slot) => setEditor({ course: null, slot })}
              edit={setCourse}
            />
          </div>
        )}
        {page === "campus" && (
          <>
            <div className="xm-campus-caption">
              <span>你的校园生活工具箱</span>
              <span>逐步接入中</span>
            </div>
            <section className="xm-campus-grid" aria-label="校园服务">
              {(["classroom", "electricity", "library"] as Feature[]).map((kind) => (
                <button
                  key={kind}
                  className={`xm-service-card xm-${kind}`}
                  disabled={kind === "library" && libraryOpening}
                  onClick={() => {
                    if (kind === "classroom") setClassroomOpen(true)
                    else if (kind === "library") void openLibrary()
                    else setFeature(kind)
                  }}
                >
                  <div>
                    <span className={kind === "electricity" ? "xm-pending" : "xm-live"}>
                      {kind === "classroom"
                        ? "教务查询"
                        : kind === "library"
                          ? "官方网页"
                          : "待接入"}
                    </span>
                    <h2>{features[kind].name}</h2>
                    <p>{features[kind].detail}</p>
                  </div>
                  <CampusArt kind={kind} />
                  <span className="xm-card-arrow">
                    <ArrowRight size={20} />
                  </span>
                </button>
              ))}
              <button
                className="xm-service-card xm-timetable-entry"
                onClick={() => navigate("timetable")}
              >
                <div>
                  <span className="xm-pending">{allCourses.length ? "离线可用" : "等待导入"}</span>
                  <h2>我的课表</h2>
                  <p>整学期，随时查看</p>
                </div>
                {ui.style !== "classic" ? <CampusThemeArt kind="timetable" glass={ui.style === "glass"} /> : <svg viewBox="0 0 300 330" fill="none" aria-hidden="true">
                  <path d="M0 285q60-33 111-6t88-5 101 6v50H0z" fill="#54ae98" />
                  <rect x="129" y="170" width="108" height="105" rx="15" fill="#e8fff5" />
                  <path d="M129 193h108" stroke="#8fcbb5" strokeWidth="3" />
                  <path
                    d="M155 158v29m56-29v29"
                    stroke="#477e73"
                    strokeWidth="8"
                    strokeLinecap="round"
                  />
                  <path
                    d="M152 213h14v13h-14zm27 0h14v13h-14zm27 0h14v13h-14zm-54 25h14v13h-14zm27 0h14v13h-14z"
                    fill="#9ac9b9"
                  />
                </svg>}
                <span className="xm-card-arrow">
                  <ArrowRight size={20} />
                </span>
              </button>
              <button className="xm-service-card xm-email" aria-label="开通学校邮箱" disabled={emailOpening} onClick={() => void openEmail()}>
                <div><span className="xm-live">官方网页</span><h2>开通学校邮箱</h2><p>你的校园专属邮箱</p></div>
                <CampusArt kind="email" />
                <span className="xm-card-arrow"><ArrowRight size={20} /></span>
              </button>
              <button className="xm-service-card xm-focus-entry" aria-label="专注时光" onClick={() => setFocusOpen(true)}>
                <div><span className="xm-live">{focus.state?.timer ? "计时中" : "离线可用"}</span><h2>专注时光</h2><p>{focus.state?.timer ? focusTime(focus.state.timer.mode === "countup" ? focus.state.timer.elapsed_ms : focus.state.timer.remaining_ms) : "一小段时间，只做一件事"}</p></div>
                <CampusArt kind="focus" />
                <span className="xm-card-arrow"><ArrowRight size={20} /></span>
              </button>
              <button className="xm-service-card xm-todos-entry" aria-label="待办事项" onClick={() => openTodos()}>
                <div><span className="xm-live">离线可用</span><h2>待办事项</h2><p>把想做的事，留在这里</p></div>
                <CampusArt kind="todos" />
                <span className="xm-card-arrow"><ArrowRight size={20} /></span>
              </button>
              <button className="xm-service-card xm-guide-entry" aria-label="校园指南" onClick={() => setGuideOpen(true)}>
                <div><span className="xm-live">官方资料</span><h2>校园指南</h2><p>查信息，逛校园，认识河大</p></div>
                <CampusArt kind="guide" />
                <span className="xm-card-arrow"><ArrowRight size={20} /></span>
              </button>
            </section>
            <p className="xm-campus-note">课表离线可用，教室联网查询，图书馆与邮箱直达官网。</p>
            {emailError && <p className="xm-notice" role="alert">{emailError}<button onClick={() => void openEmail()}>重新打开邮箱开户</button></p>}
            {libraryError && (
              <p className="xm-notice" role="alert">
                {libraryError}
                <button onClick={() => void openLibrary()}>重新打开图书馆</button>
              </p>
            )}
          </>
        )}
        {page === "me" && <MeSettings boot={boot} connection={connection} semester={semester} courseCount={allCourses.length} saved={savedTime(boot.saved_at)} todoCount={(todos.state?.counts.open ?? 0) + (todos.state?.counts.completed ?? 0)} presentation={presentation} style={ui.style} setStyle={ui.setStyle} backgroundCustom={!!background.src} openBackground={() => setBackgroundOpen(true)} openSchool={openSchool} refresh={refresh} />}
      </main>
      <MobileNavigation page={page} navigate={navigate} suspended={modalOpen || notice.open || guideOpen || focusOpen || todoOpen || schoolOpen || classroomOpen || !!course || !!editor || backgroundOpen || !!feature} />
      <FirstRunNotice notice={notice} />
      {guideOpen && <CampusGuidePage close={() => setGuideOpen(false)} workspaceId={boot.workspace_id} semester={semester} />}
      {todoOpen && <TodoPage todos={todos} createRequest={todoCreate} close={() => { setTodoOpen(false); setTodoCreate(0) }} />}
      {classroomOpen && (
        <ClassroomPage
          workspaceId={boot.workspace_id}
          semester={semester}
          revision={boot.revision}
          connection={connection}
          close={() => setClassroomOpen(false)}
          openSchool={() => {
            resumeClassroom.current = true
            setClassroomOpen(false)
            setSchoolOpen(true)
          }}
        />
      )}
      {schoolOpen && (
        <LocalSchoolPanel
          bootstrap={boot}
          semester={semester}
          close={() => {
            setSchoolOpen(false)
            void refresh()
              .catch(() => {})
              .finally(() => {
                if (resumeClassroom.current) {
                  resumeClassroom.current = false
                  setClassroomOpen(true)
                }
              })
          }}
          onSaved={async (importedSemester) => { if (importedSemester) chooseSemester(importedSemester); await refresh() }}
        />
      )}
      {feature && (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setFeature(null)
          }}
          title={features[feature].name}
          description={features[feature].subtitle}
          className="xm-feature-modal"
        >
          <div className={`xm-feature-art xm-${feature}`}>
            <CampusArt kind={feature} />
          </div>
          <span className="xm-detail-pending">待接入</span>
          <p>{features[feature].description}</p>
          <p className="xm-feature-status">{features[feature].status}</p>
          <button className="xm-primary" onClick={() => setFeature(null)}>
            知道了
          </button>
        </Modal>
      )}
      {course && (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setCourse(null)
          }}
          title={course.name}
          description="保存在本机的课程详情"
          className="xm-course-modal"
        >
          <dl>
            <div>
              <dt>
                <Clock3 size={16} />
                上课时间
              </dt>
              <dd>
                {["", "周一", "周二", "周三", "周四", "周五", "周六", "周日"][course.day_of_week]} ·{" "}
                {course.start_time}–{course.end_time}
              </dd>
            </div>
            <div>
              <dt>
                <MapPin size={16} />
                教室
              </dt>
              <dd>{course.location || "学校未提供教室"}</dd>
            </div>
            <div>
              <dt>
                <UserRound size={16} />
                教师
              </dt>
              <dd>{course.teacher || "学校未提供教师"}</dd>
            </div>
            <div>
              <dt>
                <CalendarDays size={16} />
                教学周
              </dt>
              <dd>{formatTeachingWeeks(course.week_pattern)}</dd>
            </div>
          </dl>
          <div className="xm-course-color">
            <div className="xm-course-color-title">
              <strong>课表颜色</strong>
              <span>仅影响本机显示</span>
            </div>
            <div className="xm-course-color-list" aria-label="选择课程颜色">
              {COURSE_COLORS.map((item) => (
                <button
                  key={item.value}
                  type="button"
                  className={`xm-course-color-dot ${
                    course.color.toUpperCase() === item.value.toUpperCase() ? "selected" : ""
                  }`}
                  style={{ backgroundColor: item.value }}
                  aria-label={`设为${item.label}`}
                  aria-pressed={course.color.toUpperCase() === item.value.toUpperCase()}
                  disabled={colorBusy}
                  onClick={() => void changeCourseColor(item.value)}
                />
              ))}
            </div>
            {colorError && (
              <p className="xm-course-color-error" role="alert">
                {colorError}
              </p>
            )}
          </div>
          <p className="xm-detail-note">颜色只影响本机显示，不会改动学校原始安排。</p>
          <button className="xm-primary" disabled={colorBusy} onClick={() => { setEditor({ course }); setCourse(null) }}>编辑课程信息</button>
          <button className="xm-primary" onClick={() => setCourse(null)}>
            返回课表
          </button>
        </Modal>
      )}
      {editor && <CourseEditor course={editor.course} slot={editor.slot}
        defaults={{ semester, anchor: anchor ?? "2026-08-31" }}
        local={{ workspaceId: boot.workspace_id, revision: boot.revision }}
        close={() => setEditor(null)} saved={async () => { await refresh(); setEditor(null) }} />}
      {backgroundOpen && <HomeBackgroundSettings value={background} close={() => setBackgroundOpen(false)} />}
      {focusOpen && <FocusPage focus={focus} courseNames={[...new Set(allCourses.map(c => c.name))]} close={() => setFocusOpen(false)} />}
    </div>
  )
}
