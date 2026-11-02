import { useEffect, useLayoutEffect, useRef, useState } from "react"
import { CheckCircle2, ArrowRight, RefreshCw } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import type { SemesterSettings } from "@/lib/api/academic"
import { CAMPUSES, campusPeriods, selectedCampus, semesterSettings, type Campus } from "@/lib/mobile/campus"
import { weekRanges } from "@/lib/mobile/weeks"
import { localWorkspaceBridge, localSchoolBridge, operationActive, localSchoolError, maySavePreview, unscheduledCourses,
  type LocalApi, type SchoolApi, type LocalBootstrap, type LocalConnection, type LocalOperation, type LocalPreview, type LocalReceipt } from "@/lib/mobile/local-school"
import "./local-school.css"

const weekdays = ["", "周一", "周二", "周三", "周四", "周五", "周六", "周日"]
const safeWarnings = new Set(["MISSING_LOCATION", "LEGACY_CONTENT_PRESERVED"])

export function LocalSchoolPanel({ bootstrap: initial, semester, close, onSaved, local = localWorkspaceBridge, school = localSchoolBridge }: {
  bootstrap: LocalBootstrap; semester: string; close: () => void; onSaved: (semester?: string) => Promise<void>
  local?: LocalApi; school?: SchoolApi
}) {
  const [connection, setConnection] = useState<LocalConnection | null>(null)
  const [settings, setSettings] = useState<SemesterSettings | null>(null)
  const [campus, setCampus] = useState<Campus | null>(null)
  const [anchor, setAnchor] = useState("")
  const [revision, setRevision] = useState(initial.revision)
  const [operation, setOperation] = useState<LocalOperation | null>(null)
  const [preview, setPreview] = useState<LocalPreview | null>(null)
  const [receipt, setReceipt] = useState<LocalReceipt | null>(null)
  const [skipPrompt, setSkipPrompt] = useState<LocalPreview | null>(null)
  const [error, setError] = useState("")
  const [requesting, setRequesting] = useState(false)
  const [acks, setAcks] = useState<string[]>([])
  const [checked, setChecked] = useState(false)
  const mounted = useRef(true)
  const importing = useRef(false)
  const saveCallback = useRef(onSaved)
  const commandKeys = useRef(new Map<string, string>())
  const commitKeys = useRef(new Map<string, string>())
  const commitInputs = useRef(new Map<string, Parameters<SchoolApi["commitPreview"]>[0]>())
  const pollErrors = useRef(0)
  const ws = initial.workspace_id
  useLayoutEffect(() => { saveCallback.current = onSaved }, [onSaved])
  const busy = requesting || operationActive(operation)
  const connected = connection?.connection_state === "CONNECTED" || connection?.connection_state === "TEMPORARY_FAILURE"

  useEffect(() => {
    mounted.current = true
    let cancelled = false
    async function load() {
      const conn = await school.getConnection({ workspace_id: ws })
      const target = conn.current_term?.upstream_term === "0" ? `${conn.current_term.upstream_year}S2` : semester
      const value = await local.getSettings({ workspace_id: ws, semester: target })
      if (cancelled) return
      setConnection(conn); setSettings(value.settings); setRevision(value.revision)
      setCampus(selectedCampus(value.settings))
      setAnchor(value.settings.week_anchor.start_date ?? (target === "2026S2" ? "2026-08-31" : ""))
      const active = conn.active_login_id ?? conn.active_job_id
      if (active) {
        const op = await school.getOperation({ workspace_id: ws, operation_id: active })
        if (!cancelled) setOperation(op)
      }
    }
    void load().catch(err => { if (!cancelled) setError(localSchoolError(err)) })
    return () => { cancelled = true; mounted.current = false }
  }, [ws, semester, local, school])

  useEffect(() => {
    if (!operation || !operationActive(operation)) return
    let cancelled = false
    const timer = window.setTimeout(async () => {
      let observed: LocalOperation | null = null
      try {
        const input = { workspace_id: ws, operation_id: operation.operation_id }
        const next = operation.kind === "LOGIN" ? await school.verifyLogin(input) : await school.getOperation(input)
        observed = next
        if (cancelled) return
        if (next.state === "SUCCEEDED") {
          const conn = await school.getConnection({ workspace_id: ws })
          if (cancelled) return
          setConnection(conn)
          if (next.kind === "LOGIN") {
            const target = conn.current_term?.upstream_term === "0" ? `${conn.current_term.upstream_year}S2` : semester
            const value = await local.getSettings({ workspace_id: ws, semester: target })
            if (cancelled) return
            setSettings(value.settings); setRevision(value.revision)
            setAnchor(value.settings.week_anchor.start_date ?? (target === "2026S2" ? "2026-08-31" : ""))
          }
          if (next.preview_id) {
            // Recover a lost successful commit reply using the identical key
            // before asking for a preview that may now be COMMITTED/expired.
            const pending = commitInputs.current.get(next.preview_id)
            if (importing.current && pending) {
              const result = await school.commitPreview(pending)
              if (cancelled) return
              importing.current = false; setReceipt(result); setError(""); setOperation(next)
              try { await saveCallback.current(result.semester) } catch { if (!cancelled) setError("课表已保存，请返回课表重新读取。") }
              return
            }
            const value = await school.getPreview({ workspace_id: ws, preview_id: next.preview_id })
            if (cancelled) return
            setPreview(value); setAcks([]); setChecked(false)
            if (unscheduledCourses(value).length > 0) { setSkipPrompt(value); importing.current = false }
            const warnings = [...new Set(value.issues.filter(i => i.severity === "WARNING").map(i => i.code))].sort()
            if (importing.current && warnings.every(c => safeWarnings.has(c)) && maySavePreview(value, value.base_revision, conn.connection_generation, true, warnings)) {
              const key = commitKeys.current.get(value.preview_id) ?? crypto.randomUUID()
              commitKeys.current.set(value.preview_id, key)
              const commitInput = { workspace_id: ws, preview_id: value.preview_id, expected_revision: value.base_revision, idempotency_key: key, acknowledged_warning_codes: warnings }
              commitInputs.current.set(value.preview_id, commitInput)
              const result = await school.commitPreview(commitInput)
              if (cancelled) return
              setReceipt(result); setError(""); importing.current = false
              try { await saveCallback.current(result.semester) } catch { if (!cancelled) setError("课表已保存，请返回课表重新读取。") }
            }
          }
        } else if (next.state === "FAILED" || next.state === "EXPIRED") {
          importing.current = false; setError(localSchoolError({ code: next.error?.code }))
        }
        if (!cancelled) { pollErrors.current = 0; setOperation(next) }
      } catch (err) {
        if (!cancelled) {
          setError(localSchoolError(err))
          const code = err && typeof err === "object" && "code" in err ? String(err.code) : ""
          if (++pollErrors.current >= 3 || ["REVISION_CONFLICT", "PREVIEW_EXPIRED", "INVALID_ARGUMENT", "UPSTREAM_AUTH_REQUIRED"].includes(code)) {
            importing.current = false
            setOperation(observed && !operationActive(observed) ? observed : { ...operation, state: "FAILED" })
          } else setOperation(old => old ? { ...old } : old)
        }
      }
    }, 900)
    return () => { cancelled = true; window.clearTimeout(timer) }
  }, [operation, ws, semester, local, school])

  async function act(action: () => Promise<void>) {
    if (requesting) return
    setRequesting(true); setError("")
    try { await action() } catch (err) { if (mounted.current) setError(localSchoolError(err)) }
    finally { if (mounted.current) setRequesting(false) }
  }
  async function login() {
    pollErrors.current = 0
    const key = commandKeys.current.get("login") ?? crypto.randomUUID()
    commandKeys.current.set("login", key)
    const op = await school.startLogin({ workspace_id: ws, idempotency_key: key })
    commandKeys.current.delete("login")
    if (mounted.current) setOperation(op)
  }
  async function importCourses(selected: Campus) {
    if (!settings || !anchor) return
    importing.current = true; pollErrors.current = 0; setPreview(null); setSkipPrompt(null)
    const current = await local.getSettings({ workspace_id: ws, semester: settings.semester })
    const configured = semesterSettings(current.settings, selected, anchor)
    const value = await local.putSettings({ workspace_id: ws, expected_revision: current.revision, settings: configured })
    if (!mounted.current) return
    setSettings(value.settings); setRevision(value.revision)
    const digest = `fetch:${settings.semester}:${value.revision}`
    const key = commandKeys.current.get(digest) ?? crypto.randomUUID()
    commandKeys.current.set(digest, key)
    const op = await school.fetchPreview({ workspace_id: ws, semester: settings.semester, expected_revision: value.revision, idempotency_key: key })
    commandKeys.current.delete(digest)
    if (mounted.current) setOperation(op)
  }
  const warningCodes = [...new Set(preview?.issues.filter(i => i.severity === "WARNING").map(i => i.code) ?? [])].sort()
  const skipped = unscheduledCourses(skipPrompt)
  const skipWarnings = [...new Set(skipPrompt?.issues.filter(i => i.severity === "WARNING").map(i => i.code) ?? [])].sort()
  const canSkipAndSave = skipWarnings.every(code => safeWarnings.has(code) || code === "UNSCHEDULED_COURSE") && maySavePreview(skipPrompt, revision, connection?.connection_generation, true, skipWarnings)
  return <><Modal open onOpenChange={open => { if (!open && !busy) close() }} title="学校连接与更新" description="登录账号，选择校区，课表自动存到手机。" className="xq-henu-modal xq-local-school">
    {error && <p role="alert" className="xq-henu-message error">{error}</p>}
    {receipt ? <section className="xql-success" role="status"><CheckCircle2 size={35} /><h3>{receipt.no_changes ? "课表已是最新" : "课表已保存到手机"}</h3><p>整学期离线可用。你的个人修改继续保留。</p>{receipt.warning_codes.includes("MISSING_LOCATION") && <p>部分课程学校未提供教室，可在课表中自行补充。</p>}<button className="xq-button primary" onClick={close}>返回课表<ArrowRight size={16} /></button></section> : <>
      <section className="xql-step"><div className="xql-step-title"><b>1</b><h3>登录学校账号</h3>{connected && <CheckCircle2 size={19} />}</div><p>账号、密码和验证码在学校官方页面填写。</p>
        <button className="xq-button primary" disabled={busy || connected || !connection?.capabilities.interactive_login} onClick={() => void act(login)}>{connected ? "已登录，可直接继续" : "登录学校账号"}<ArrowRight size={16} /></button>
      </section>
      <section className="xql-step"><div className="xql-step-title"><b>2</b><h3>选择你的校区</h3></div><p>点击校区后，自动按内置的 13 节作息导入课表。</p>
        <div className="xql-campuses" role="group" aria-label="选择校区">{CAMPUSES.map(c => <button type="button" key={c} disabled={busy || !connected || !settings || !anchor} aria-label={`${c}校区`} aria-pressed={campus === c} className={campus === c ? "selected" : ""} onClick={() => { setCampus(c); setPreview(null); void act(() => importCourses(c)) }}>{c}校区<small>选择并导入</small></button>)}</div>
        {campus && <details className="xql-settings"><summary>查看{campus}校区作息</summary><div className="xql-periods">{campusPeriods(campus).map(p => <p key={p.index}>第 {p.index} 节 <strong>{p.start_time}–{p.end_time}</strong></p>)}</div><p>金明与明伦校区使用相同作息。</p></details>}
        {settings && <details className="xql-settings" open={!anchor}><summary>学期信息：{settings.semester_label || settings.semester}</summary><p>共 18 周 · 第一周周一 {anchor || "待设置"}</p><label>第一周周一<input type="date" value={anchor} disabled={busy} onChange={e => { setAnchor(e.target.value); setPreview(null) }} /></label><p>内置已确认的 2026 年秋季开学日期；其他学期请按校历填写。</p></details>}
        {requesting && <p role="status"><RefreshCw size={15} />正在准备导入…</p>}
        <p className="xql-hint">学校未提供的教室可稍后补充。现有课程和个人修改会保留。更新或重试时，再次点击校区即可。</p>
      </section>
      {operationActive(operation) && <div role="status" className="xql-hint">{operation?.kind === "LOGIN" ? "请完成学校登录，返回后继续选校区。" : "正在读取并保存，请稍候…"}<button className="xq-text-button" disabled={requesting} onClick={() => void act(async () => { if (!operation) return; importing.current = false; await school.cancelOperation({ workspace_id: ws, operation_id: operation.operation_id }); setOperation(await school.getOperation({ workspace_id: ws, operation_id: operation.operation_id })) })}>取消</button></div>}
      {preview && !receipt && <section className="xql-preview"><h3>课程读取结果</h3><div className="xql-counts"><div><strong>{preview.courses.filter(c => c.sessions.length > 0).length}</strong><span>门有排课课程</span></div><div><strong>{preview.stats.sessions}</strong><span>个安排</span></div><div><strong>{preview.diff.unchanged.length}</strong><span>未变化</span></div></div>
        {(preview.status !== "READY" || !preview.can_commit) && <p className="xql-hint">本次学校课程发生变化或有信息需要核对，暂未覆盖已有课表。</p>}
        {preview.issues.filter(i => i.severity === "BLOCKING").map((i, index) => <p className="xql-hint" key={`${i.code}-${index}`}>{i.message}</p>)}
        <details><summary>查看课程、节次与周次</summary><div className="xql-rows">{preview.courses.map(c => <article key={c.preview_course_id}><strong>{c.name}</strong>{c.sessions.map(s => <p key={s.preview_session_id}>{weekdays[s.weekday]} · 第 {weekRanges(s.sections)} 节 · {weekRanges(s.weeks)}周<br />{s.start_time}–{s.end_time} · {s.location || "未提供教室"}</p>)}</article>)}</div></details>
        {unscheduledCourses(preview).length > 0 && !busy && <button className="xq-button" onClick={() => setSkipPrompt(preview)}>查看未排课课程并继续</button>}
        {preview.status === "READY" && !busy && unscheduledCourses(preview).length === 0 && <>{warningCodes.map(code => <label className="xql-check" key={code}><input type="checkbox" checked={acks.includes(code)} onChange={e => setAcks(old => e.target.checked ? [...old, code] : old.filter(c => c !== code))} />{preview.issues.find(i => i.code === code)?.message}</label>)}<label className="xql-check"><input type="checkbox" checked={checked} onChange={e => setChecked(e.target.checked)} />我已核对课程</label><button className="xq-button primary" disabled={!maySavePreview(preview, revision, connection?.connection_generation, checked, acks)} onClick={() => void act(async () => {
          const key = commitKeys.current.get(preview.preview_id) ?? crypto.randomUUID(); commitKeys.current.set(preview.preview_id, key)
          const result = await school.commitPreview({ workspace_id: ws, preview_id: preview.preview_id, expected_revision: revision, idempotency_key: key, acknowledged_warning_codes: [...new Set(acks)].sort() })
          if (!mounted.current) return; setReceipt(result); await saveCallback.current(result.semester)
        })}>保存到手机</button></>}
      </section>}
      {connected && <details className="xql-settings"><summary>账号管理</summary><button className="xq-text-button" disabled={busy} onClick={() => void act(login)}>重新登录学校账号</button><button className="xq-text-button" disabled={busy} onClick={() => void act(async () => { if (!connection) return; await school.disconnect({ workspace_id: ws, expected_connection_generation: connection.connection_generation }); setConnection(await school.getConnection({ workspace_id: ws })); setPreview(null) })}>断开学校连接（保留本机课表）</button></details>}
    </>}
  </Modal>
    {skipPrompt && <Modal open title="这些课程不加入课表" description="没有具体上课时间的课程不会占用课表格子。" className="xm-unscheduled-modal" onOpenChange={open => { if (!open && !requesting) setSkipPrompt(null) }}>
      <p>以下 {skipped.length} 门课程没有星期和上课节次，可能是网课或尚未排课。继续后将跳过它们，仅保存有上课安排的课程。</p>
      <ul>{skipped.map(c => <li key={c.preview_course_id}><strong>{c.name}</strong>{c.upstream_course_code && <small>{c.upstream_course_code}</small>}<span>无上课时间 · 不加入课表</span></li>)}</ul>
      <p>{skipPrompt.stats.sessions > 0 ? `其余 ${skipPrompt.courses.filter(c => c.sessions.length > 0).length} 门课程，共 ${skipPrompt.stats.sessions} 个上课安排。` : "本次没有可导入的上课安排，现有课表继续保留。"}</p>
      {!canSkipAndSave && skipPrompt.stats.sessions > 0 && <p className="xql-hint">其他课程还有待核对信息，请返回查看。现有课表不会被覆盖。</p>}
      {error && <p role="alert" className="xq-form-error">{error}</p>}
      {canSkipAndSave && <button className="xm-primary" disabled={busy} onClick={() => void act(async () => {
        const value = skipPrompt
        const key = commitKeys.current.get(value.preview_id) ?? crypto.randomUUID(); commitKeys.current.set(value.preview_id, key)
        const result = await school.commitPreview({ workspace_id: ws, preview_id: value.preview_id, expected_revision: revision, idempotency_key: key, acknowledged_warning_codes: skipWarnings })
        if (!mounted.current) return
        setReceipt(result); setSkipPrompt(null); importing.current = false
        try { await saveCallback.current(result.semester) } catch { if (mounted.current) setError("课表已保存，请返回课表重新读取。") }
      })}>{requesting ? "正在导入…" : "跳过并导入其余课程"}</button>}
      <button className="xq-button" disabled={requesting} onClick={() => setSkipPrompt(null)}>{canSkipAndSave ? "暂不导入" : "返回核对"}</button>
    </Modal>}
  </>
}
