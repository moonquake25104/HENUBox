import { useEffect, useRef, useState } from "react"
import {
  cancelHenuLogin,
  disconnectHenu,
  getHenuConnection,
  getHenuJob,
  getHenuLogin,
  jobActive,
  loginActive,
  startHenuJob,
  startHenuLogin,
  type HenuConnectionState,
  type HenuJob,
  type HenuLogin,
  type HenuSyncRequest,
} from "@/lib/api/henu-connection"
import { getAcademicTimetable } from "@/lib/api/academic"
import { userErrorMessage } from "@/lib/errors"
import { ApiRequestError } from "@/lib/api/client"

export function useHenuConnection() {
  const [connection, setConnection] = useState<HenuConnectionState | null>(null)
  const [login, setLogin] = useState<HenuLogin | null>(null)
  const [job, setJob] = useState<HenuJob | null>(null)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)
  const [requesting, setRequesting] = useState(false)
  const [epoch, setEpoch] = useState(0)
  const ids = useRef({ login: "", job: "" })
  const pendingLogin = useRef<string | null>(null)
  const pendingJob = useRef<HenuSyncRequest | null>(null)
  const locked = useRef(false)
  const live = useRef(true)
  useEffect(() => {
    live.current = true
    return () => {
      live.current = false
    }
  }, [])
  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      try {
        const value = await getHenuConnection()
        const loginId = value.active_login_id || ids.current.login
        const jobId = value.active_job_id || ids.current.job
        const absent = (err: unknown) => {
          if (err instanceof ApiRequestError && err.status === 404) return null
          throw err
        }
        const [nextLogin, nextJob] = await Promise.all([
          loginId ? getHenuLogin(loginId).catch(absent) : Promise.resolve(null),
          jobId ? getHenuJob(jobId).catch(absent) : Promise.resolve(null),
        ])
        if (cancelled) return
        ids.current = { login: nextLogin ? loginId : "", job: nextJob ? jobId : "" }
        setConnection(value)
        setLogin(nextLogin)
        setJob(nextJob)
        // A server-recognized task settles a lost POST response without opening another window.
        if (value.active_login_id) pendingLogin.current = null
        if (value.active_job_id) pendingJob.current = null
        setError("")
        setLoading(false)
        if (loginActive(nextLogin) || jobActive(nextJob)) timer = setTimeout(poll, 2000)
      } catch (err) {
        if (cancelled) return
        setLoading(false)
        setError(userErrorMessage(err, "连接状态暂时无法读取，请重试"))
        // Keep observing an existing task after a transient polling failure.
        if (ids.current.login || ids.current.job) timer = setTimeout(poll, 4000)
      }
    }
    void poll()
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [epoch])

  async function action(work: () => Promise<void>) {
    if (locked.current) return
    locked.current = true
    setRequesting(true)
    setError("")
    try {
      await work()
    } catch (err) {
      // An explicit rejection is safe to correct and retry with a fresh request.
      // Transport/5xx uncertainty retains the exact original request and key.
      if (err instanceof ApiRequestError && err.status >= 400 && err.status < 500) {
        pendingLogin.current = null
        pendingJob.current = null
      }
      if (live.current)
        setError(
          err instanceof TypeError
            ? "网络连接中断，操作结果尚未确认。请重试，将沿用同一次请求。"
            : userErrorMessage(err, "操作未能确认，请重试"),
        )
    } finally {
      locked.current = false
      if (live.current) setRequesting(false)
    }
  }
  const active = loginActive(login) || jobActive(job)
  return {
    connection,
    login,
    job,
    error,
    loading,
    requesting,
    active,
    refresh: () => setEpoch((value) => value + 1),
    connect: () =>
      action(async () => {
        if (!pendingLogin.current) {
          ids.current.login = ""
          setLogin(null)
        }
        pendingLogin.current ??= crypto.randomUUID()
        const value = await startHenuLogin(pendingLogin.current)
        pendingLogin.current = null
        if (!live.current) return
        ids.current.login = value.login_id
        setLogin(value)
        setEpoch((value) => value + 1)
      }),
    check: (semester: string) =>
      action(async () => {
        // Preserve BOTH version and key across an uncertain POST outcome.
        if (!pendingJob.current) {
          ids.current.job = ""
          setJob(null)
          const timetable = await getAcademicTimetable(semester)
          pendingJob.current = {
            semester,
            expected_aggregate_revision: timetable.aggregate_revision,
            idempotency_key: crypto.randomUUID(),
          }
        }
        const value = await startHenuJob(pendingJob.current)
        pendingJob.current = null
        if (!live.current) return
        ids.current.job = value.job_id
        setJob(value)
        setEpoch((value) => value + 1)
      }),
    cancel: () =>
      action(async () => {
        if (!login) return
        await cancelHenuLogin(login.login_id)
        if (live.current) setEpoch((value) => value + 1)
      }),
    disconnect: () =>
      action(async () => {
        await disconnectHenu()
        pendingLogin.current = null
        pendingJob.current = null
        if (!live.current) return
        ids.current = { login: "", job: "" }
        setLogin(null)
        setJob(null)
        setConnection(null)
        setEpoch((value) => value + 1)
      }),
  }
}
export type HenuController = ReturnType<typeof useHenuConnection>
export function henuStatusLabel(value: HenuConnectionState | null) {
  if (!value) return "连接状态待确认"
  return (
    {
      UNAVAILABLE: "当前服务未启用直连",
      DISCONNECTED: "尚未连接河大",
      CONNECTED: "河大已连接",
      AUTH_REQUIRED: "需要重新登录",
      TEMPORARY_FAILURE: "学校暂时无法访问",
    }[value.connection_state] || "连接状态待确认"
  )
}
