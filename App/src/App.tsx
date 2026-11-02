import { useEffect, useState } from "react"
import { clearToken, getToken } from "@/lib/api/client"
import { initializeSession } from "@/lib/mobile/session"
import { nativeApp } from "@/lib/mobile/platform"
import { SessionGate } from "@/components/auth/SessionGate"
import { LoginPage } from "@/components/auth/LoginPage"
import { XiqueShell } from "@/components/xique/XiqueShell"
import { Dashboard } from "@/components/xique/Dashboard"
import { CoursesPage } from "@/components/xique/CoursesPage"
import { SettingsPage } from "@/components/xique/SettingsPage"
import { CourseEditor } from "@/components/xique/CourseEditor"
import { HenuConnection } from "@/components/xique/HenuConnection"
import { HenuImport } from "@/components/xique/HenuImport"
import { henuStatusLabel, useHenuConnection } from "@/components/xique/use-henu-connection"
import { useCourseData } from "@/components/xique/use-course-data"
import { currentSemester, defaultAnchor } from "@/components/xique/dates"
import type { Course } from "@/types/course"
import { MobileApp } from "@/components/mobile/MobileApp"

export type Page = "today" | "courses" | "settings"
export interface CourseDefaults {
  semester: string
  anchor: string
}
function savedDefaults(): CourseDefaults {
  try {
    const value = JSON.parse(
      localStorage.getItem("xique-course-defaults") ?? "null",
    ) as CourseDefaults | null
    if (
      value?.semester?.trim() &&
      /^\d{4}-\d{2}-\d{2}$/.test(value.anchor) &&
      Number.isFinite(Date.parse(value.anchor))
    )
      return value
  } catch {
    /* First-run defaults also work if storage is unavailable. */
  }
  return { semester: currentSemester(), anchor: defaultAnchor() }
}
function pageFromHash(): Page {
  const value = location.hash.replace("#/", "")
  return value === "courses" || value === "settings" ? value : "today"
}
function Workspace() {
  const data = useCourseData()
  const henu = useHenuConnection()
  const [connectionOpen, setConnectionOpen] = useState(false)
  const [directPreview, setDirectPreview] = useState<{ id: string; semester: string } | null>(null)
  const schoolStatus = henuStatusLabel(henu.connection)
  const openSchool = () => {
    setConnectionOpen(true)
    henu.refresh()
  }
  const [page, setPage] = useState<Page>(pageFromHash)
  const [defaults, setDefaults] = useState<CourseDefaults>(savedDefaults)
  const [semester, setSemesterState] = useState(() => {
    try {
      return localStorage.getItem("xique-active-semester") || defaults.semester
    } catch {
      return defaults.semester
    }
  })
  const [editor, setEditor] = useState<{ course: Course | null } | null>(null)
  const [notice, setNotice] = useState("")
  useEffect(() => {
    const changed = () => setPage(pageFromHash())
    window.addEventListener("hashchange", changed)
    return () => window.removeEventListener("hashchange", changed)
  }, [])
  useEffect(() => {
    if (!notice) return
    const timer = window.setTimeout(() => setNotice(""), 4500)
    return () => clearTimeout(timer)
  }, [notice])
  const navigate = (next: Page) => {
    location.hash = `/${next}`
    setPage(next)
  }
  const semesters = [
    ...new Set([
      semester,
      defaults.semester,
      currentSemester(),
      ...data.courses.map((course) => course.semester),
    ]),
  ].filter(Boolean)
  const setSemester = (value: string) => {
    setSemesterState(value)
    try {
      localStorage.setItem("xique-active-semester", value)
    } catch {
      /* Selecting a semester still works without persistent browser storage. */
    }
  }
  const courses = data.courses.filter((course) => course.semester === semester)
  const anchor =
    courses.find((course) => course.semester_start_date)?.semester_start_date || defaults.anchor
  function saveDefaults(value: CourseDefaults) {
    localStorage.setItem("xique-course-defaults", JSON.stringify(value))
    setDefaults(value)
    setSemester(value.semester)
    setNotice("新建课程的默认设置已保存")
  }
  const nativeDataUnavailable = nativeApp && !data.loading && Boolean(data.error)
  return (
    <XiqueShell page={page} navigate={navigate} connected={data.connected} localSaved={nativeApp && !!data.readAt}>
      {nativeApp && data.readAt && (
        <section className="xq-local-timetable" aria-label="本机课表状态">
          <div>
            <strong>整学期课表已存本机</strong>
            <p>{data.connected ? "已同步" : "正在使用本机课表"} · {data.readAt.toLocaleString("zh-CN", { timeZone: "Asia/Shanghai", month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" })}</p>
            {data.syncError && <p role="status">{data.syncError}</p>}
          </div>
          <button className="xq-button small" disabled={data.refreshing} onClick={() => void data.refresh()}>
            {data.refreshing ? "正在同步…" : "更新本机课表"}
          </button>
        </section>
      )}
      {nativeDataUnavailable && page !== "settings" && (
        <section className="xq-panel xq-day-off">
          <h2>暂时读不到课表</h2>
          <p>手机尚未保存完整课表。请先连接HENUBox 服务并读取一次；保存后即可离线查看。</p>
          <p role="alert">{data.error}</p>
          <button className="xq-button primary" onClick={() => void data.refresh()}>
            重新读取课表
          </button>
          <button className="xq-text-button" onClick={() => navigate("settings")}>
            检查服务地址
          </button>
        </section>
      )}
      {page === "today" && !nativeDataUnavailable && (
        <Dashboard
          semester={semester}
          setSemester={setSemester}
          semesters={semesters}
          anchor={anchor}
          courses={courses}
          revision={data.readAt}
          error={data.error}
          refresh={data.refresh}
          navigate={navigate}
          schoolStatus={schoolStatus}
          openSchool={openSchool}
          add={() => setEditor({ course: null })}
          edit={(course) => setEditor({ course })}
        />
      )}
      {page === "courses" && !nativeDataUnavailable && (
        <CoursesPage
          semester={semester}
          setSemester={setSemester}
          semesters={semesters}
          anchor={anchor}
          courses={courses}
          revision={data.readAt}
          loading={data.loading}
          error={data.error}
          refresh={data.refresh}
          openSchool={openSchool}
          add={() => setEditor({ course: null })}
          edit={(course) => setEditor({ course })}
        />
      )}
      {page === "settings" && (
        <SettingsPage
          defaults={defaults}
          saveDefaults={saveDefaults}
          connected={data.connected}
          readAt={data.readAt}
          schoolStatus={schoolStatus}
          openSchool={openSchool}
        />
      )}
      {connectionOpen && (
        <HenuConnection
          controller={henu}
          semester={semester}
          close={() => setConnectionOpen(false)}
          preview={(id, target) => {
            setConnectionOpen(false)
            setDirectPreview({ id, semester: target })
          }}
        />
      )}
      {directPreview && (
        <HenuImport
          key={directPreview.id}
          semester={directPreview.semester}
          initialPreviewId={directPreview.id}
          onCommitted={data.refresh}
          close={() => setDirectPreview(null)}
        />
      )}
      {editor && (
        <CourseEditor
          course={editor.course}
          defaults={{ semester, anchor }}
          close={() => setEditor(null)}
          saved={async (message) => {
            setEditor(null)
            setNotice(message)
            await data.refresh()
          }}
        />
      )}
      {notice && (
        <div className="xq-toast" role="status">
          {notice}
        </div>
      )}
    </XiqueShell>
  )
}
function DesktopApp() {
  const [authed, setAuthed] = useState(() => getToken() !== null)
  const [booted, setBooted] = useState(!nativeApp)
  const [bootError, setBootError] = useState("")
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    if (!nativeApp) return
    let live = true
    initializeSession()
      .then(() => {
        if (live) {
          setAuthed(getToken() !== null)
          setBooted(true)
          setBootError("")
        }
      })
      .catch((err) => {
        if (live) {
          setBootError(err instanceof Error ? err.message : "安全登录信息读取失败")
          setBooted(false)
        }
      })
    return () => {
      live = false
    }
  }, [attempt])
  useEffect(() => {
    const unauthorized = () => setAuthed(false)
    const failed = () => {
      setBootError("本机登录未能清理，请重试清除后再登录。")
      setBooted(false)
    }
    const changed = () => {
      setAuthed(false)
    }
    window.addEventListener("kairos:unauthorized", unauthorized)
    window.addEventListener("xique:session-storage-error", failed)
    window.addEventListener("xique:server-changed", changed)
    return () => {
      window.removeEventListener("kairos:unauthorized", unauthorized)
      window.removeEventListener("xique:session-storage-error", failed)
      window.removeEventListener("xique:server-changed", changed)
    }
  }, [])
  if (!booted)
    return (
      <SessionGate
        error={bootError}
        retry={() => {
          setBootError("")
          setAttempt((value) => value + 1)
        }}
        reset={() => {
          void clearToken()
            .then(() => {
              setBooted(true)
              setBootError("")
              setAuthed(false)
            })
            .catch(() => setBootError("清除未完成，请重试。"))
        }}
      />
    )
  return authed ? <Workspace /> : <LoginPage onSuccess={() => setAuthed(true)} />
}

export default function App() {
  return nativeApp ? <MobileApp /> : <DesktopApp />
}
