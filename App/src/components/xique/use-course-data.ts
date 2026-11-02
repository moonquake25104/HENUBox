import { useCallback, useEffect, useRef, useState } from "react"
import { listCourses, getWeekSchedule } from "@/lib/api/courses"
import { nativeApp } from "@/lib/mobile/platform"
import { assertSession, sessionEpoch } from "@/lib/mobile/session"
import { localWeek, offlineTimetable, saveOfflineTimetable } from "@/lib/mobile/offline-timetable"
import { userErrorMessage } from "@/lib/errors"
import type { Course } from "@/types/course"
import type { WeekScheduleResponse } from "@/types/schedule"

export function useCourseData() {
  const [courses, setCourses] = useState<Course[]>(() => nativeApp ? offlineTimetable()?.courses ?? [] : [])
  const [loading, setLoading] = useState(() => !nativeApp || !offlineTimetable())
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState("")
  const [syncError, setSyncError] = useState("")
  const [connected, setConnected] = useState(false)
  const [readAt, setReadAt] = useState<Date | null>(() => {
    const saved = nativeApp ? offlineTimetable() : null
    return saved ? new Date(saved.saved_at) : null
  })
  const generation = useRef(0)
  const refresh = useCallback(async () => {
    const request = ++generation.current
    const account = sessionEpoch()
    setRefreshing(true)
    if (!nativeApp || !offlineTimetable()) setLoading(true)
    try {
      if (nativeApp && !navigator.onLine) throw new Error("当前离线，正在使用本机课表。")
      const data = await listCourses()
      assertSession(account)
      if (request !== generation.current) return
      const saved = nativeApp ? await saveOfflineTimetable(data) : null
      assertSession(account)
      if (request !== generation.current) return
      setCourses(saved?.courses ?? data)
      setReadAt(saved ? new Date(saved.saved_at) : new Date())
      setConnected(true)
      setError("")
      setSyncError("")
    } catch (err) {
      if (request === generation.current) {
        setConnected(false)
        const message = userErrorMessage(err, "课程读取失败，请稍后重试")
        if (nativeApp && offlineTimetable()) { setSyncError(message); setError("") }
        else setError(message)
      }
    } finally {
      if (request === generation.current) { setLoading(false); setRefreshing(false) }
    }
  }, [])
  useEffect(() => {
    let live = true
    queueMicrotask(() => { if (live) void refresh() })
    const cancel = () => { generation.current++ }
    const focused = () => {
      void refresh()
    }
    window.addEventListener("focus", focused)
    window.addEventListener("online", focused)
    const offline = () => { setConnected(false) }
    window.addEventListener("offline", offline)
    return () => {
      live = false
      cancel()
      window.removeEventListener("focus", focused)
      window.removeEventListener("online", focused)
      window.removeEventListener("offline", offline)
    }
  }, [refresh])
  return { courses, loading, refreshing, error, syncError, connected, refresh, readAt }
}
export function useWeek(semester: string, anchor: string, week: number, revision: Date | null, courses: Course[]) {
  const [data, setData] = useState<WeekScheduleResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    if (nativeApp) return
    let live = true
    Promise.resolve().then(() => {
      if (!live) return undefined
      setLoading(true)
      setError("")
      setData(null)
      return getWeekSchedule({ semester, semester_start_date: anchor, week_index: week })
    })
      .then((value) => {
        if (live && value) setData(value)
      })
      .catch((err) => {
        if (live) setError(userErrorMessage(err, "课表加载失败"))
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [semester, anchor, week, revision, retry])
  if (nativeApp) {
    try { return { data: localWeek(courses, semester, anchor, week), loading: false, error: "", retry: () => {} } }
    catch { return { data: null, loading: false, error: "学期日期或周次无效，请核对。", retry: () => {} } }
  }
  return { data, loading, error, retry: () => setRetry((value) => value + 1) }
}
