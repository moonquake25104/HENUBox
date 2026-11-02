import { useEffect, useRef, useState } from "react"
import { CheckCircle2 } from "lucide-react"
import {
  commitAcademicPreview,
  getAcademicReceipt,
  getAcademicTimetable,
  rebaseAcademicPreview,
  type AcademicPreview,
  type AcademicReceipt,
  type AcademicResolution,
  type AcademicTimetable,
} from "@/lib/api/academic"
import { userErrorMessage } from "@/lib/errors"
import { weekdays } from "./dates"

export function HenuConfirmation({
  preview,
  semester,
  busy,
  dirty,
  setBusy,
  onPreview,
  onCommitted,
  onDone,
}: {
  preview: AcademicPreview
  semester: string
  busy: boolean
  dirty: boolean
  setBusy: (value: boolean) => void
  onPreview: (value: AcademicPreview) => void
  onCommitted: () => Promise<void>
  onDone: () => void
}) {
  const [error, setError] = useState("")
  const [receipt, setReceipt] = useState<AcademicReceipt | null>(null)
  const [ack, setAck] = useState<string[]>([])
  const [reviewed, setReviewed] = useState(false)
  const [mapping, setMapping] = useState(false)
  const [choices, setChoices] = useState<Record<string, AcademicResolution>>({})
  const [choicesDirty, setChoicesDirty] = useState(false)
  const [timetable, setTimetable] = useState<AcademicTimetable | null>(null)
  const [loadError, setLoadError] = useState("")
  const [loadAttempt, setLoadAttempt] = useState(0)
  const [uncertain, setUncertain] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  const request = useRef<{
    id: string
    body: {
      expected_aggregate_revision: number
      idempotency_key: string
      acknowledged_warning_codes: string[]
    }
  } | null>(null)
  const diff = preview.diff
  const warnings = [
    ...new Set((preview.issues ?? []).filter((i) => i.severity === "WARNING").map((i) => i.code)),
  ].sort()
  const stale = preview.validation?.state !== "FRESH"
  const needsMapping = (preview.issues ?? []).some((i) =>
    ["SEMESTER_MAPPING_UNVERIFIED", "SEMESTER_MAPPING_REQUIRED"].includes(i.code),
  )
  const expired = new Date(preview.expires_at).getTime() <= now
  const revision = preview.validation?.current_aggregate_revision
  const frozen = busy || uncertain || receipt !== null
  useEffect(() => {
    let live = true
    getAcademicTimetable(semester)
      .then((value) => {
        if (live) setTimetable(value)
      })
      .catch((err) => {
        if (live) setLoadError(userErrorMessage(err, "读取已有课表失败"))
      })
    return () => {
      live = false
    }
  }, [semester, preview.preview_id, loadAttempt])
  useEffect(() => {
    if (preview.status !== "COMMITTED" || !preview.commit_id) return
    let live = true
    getAcademicReceipt(preview.commit_id)
      .then((value) => {
        if (live) setReceipt(value)
      })
      .catch((err) => {
        if (live) setError(userErrorMessage(err, "读取导入回执失败"))
      })
    return () => {
      live = false
    }
  }, [preview.status, preview.commit_id])
  function choose(key: string, value: AcademicResolution | null) {
    setChoices((old) => {
      const next = { ...old }
      if (value) next[key] = value
      else delete next[key]
      return next
    })
    setReviewed(false)
    setChoicesDirty(true)
  }
  function sessionLabel(id: string) {
    for (const course of timetable?.courses ?? []) {
      const s = course.sessions.find((s) => s.session_id === id)
      if (s)
        return `${course.name} · ${weekdays[s.source_values.weekday - 1]} · ${s.source_values.sections.join("、")}节 · ${s.source_values.location || "教室待定"}${s.active ? "" : "（已停用）"}`
    }
    return "已有安排（请先读取课表详情）"
  }
  async function rebuild() {
    if (busy || uncertain || revision === undefined) return
    setBusy(true)
    setError("")
    try {
      const next = await rebaseAcademicPreview(
        preview.preview_id,
        revision,
        Object.values(choices),
        mapping,
      )
      onPreview(next)
      setAck([])
      setReviewed(false)
      setChoicesDirty(false)
      request.current = null
    } catch (err) {
      setError(userErrorMessage(err, "更新预览失败，请检查状态后重试"))
    } finally {
      setBusy(false)
    }
  }
  async function submit() {
    if (busy || receipt) return
    setBusy(true)
    setError("")
    try {
      if (!request.current) {
        if (
          !preview.can_commit ||
          stale ||
          dirty ||
          !reviewed ||
          revision === undefined ||
          !warnings.every((code) => ack.includes(code))
        )
          return
        request.current = {
          id: preview.preview_id,
          body: {
            expected_aggregate_revision: revision,
            idempotency_key: crypto.randomUUID(),
            acknowledged_warning_codes: warnings,
          },
        }
      }
      const result = await commitAcademicPreview(request.current.id, request.current.body)
      setReceipt(result)
      setUncertain(false)
      onPreview({ ...preview, status: "COMMITTED", commit_id: result.commit_id })
      await onCommitted()
    } catch (err) {
      // The server may have committed before the connection failed. Keep the exact request.
      setUncertain(request.current !== null)
      setError(
        `${userErrorMessage(err, "提交结果尚未确认")}。可使用原请求重试；不会再次导入同一预览。`,
      )
    } finally {
      setBusy(false)
    }
  }
  const pendingChoices = choicesDirty
  const canSubmit =
    preview.can_commit &&
    !stale &&
    !expired &&
    !dirty &&
    reviewed &&
    warnings.every((code) => ack.includes(code)) &&
    !pendingChoices &&
    revision !== undefined
  return (
    <section className="xq-henu-confirmation" aria-label="确认导入">
      {error && (
        <p className="xq-henu-message error" role="alert">
          {error}
        </p>
      )}
      {receipt ? (
        <div className="xq-henu-success" role="status">
          <CheckCircle2 size={28} />
          <h3>{receipt.no_changes ? "课表已是最新，无需重复添加" : "课表已导入"}</h3>
          <p>
            新增 {receipt.counts.sessions_created} 个安排 · 更新 {receipt.counts.sessions_updated}{" "}
            个 · 停用 {receipt.counts.sessions_retired} 个
          </p>
          <p>关闭窗口即可查看课表。本次为文件导入，尚未连接学校自动更新。</p>
          <button className="xq-button primary" onClick={onDone}>
            返回课表
          </button>
        </div>
      ) : preview.status === "COMMITTED" ? (
        <p role="status">这份预览已提交，正在读取回执。关闭窗口后可刷新课表。</p>
      ) : (
        <>
          <h3>这次导入会发生什么</h3>
          {diff ? (
            <div className="xq-henu-diff" aria-label="导入变化">
              <span>
                新增 <b>{diff.added?.length ?? 0}</b>
              </span>
              <span>
                更新 <b>{diff.changed?.length ?? 0}</b>
              </span>
              <span>
                未变 <b>{diff.unchanged?.length ?? 0}</b>
              </span>
              <span>
                本次未出现 <b>{diff.missing?.length ?? 0}</b>
              </span>
            </div>
          ) : (
            <p>尚未取得完整变化信息，请检查预览状态。</p>
          )}
          {diff?.changed?.map((item) => (
            <p key={item.session_id} className="xq-henu-change-note">
              {sessionLabel(item.session_id)}：学校安排有变化，请对照上方新安排核对。
            </p>
          ))}
          {(stale ||
            needsMapping ||
            (diff?.missing?.length ?? 0) > 0 ||
            (diff?.ambiguous?.length ?? 0) > 0 ||
            (diff?.added?.length ?? 0) > 0) && (
            <details
              className="xq-henu-decisions"
              open={
                stale ||
                needsMapping ||
                (diff?.ambiguous?.length ?? 0) > 0 ||
                diff?.missing?.some((m) => !m.decision) ||
                undefined
              }
            >
              <summary>核对与已有课表的关系</summary>
              {loadError && (
                <p role="alert">
                  {loadError}{" "}
                  <button
                    className="xq-button small"
                    disabled={busy}
                    onClick={() => {
                      setLoadError("")
                      setLoadAttempt((n) => n + 1)
                    }}
                  >
                    重试读取
                  </button>
                </p>
              )}
              <fieldset disabled={frozen}>
                {needsMapping && (
                  <label className="xq-henu-confirm">
                    <input
                      type="checkbox"
                      checked={mapping}
                      onChange={(e) => {
                        setMapping(e.target.checked)
                        setChoicesDirty(true)
                      }}
                    />
                    确认文件中的学校学期对应当前目标学期
                  </label>
                )}
                {preview.courses
                  .filter((c) => !c.academic_course_id && (timetable?.courses.length ?? 0) > 0)
                  .map((c) => (
                    <label className="xq-henu-choice" key={c.preview_course_id}>
                      {c.name} · 是否为已有课程？
                      <select
                        value={choices[`course:${c.preview_course_id}`]?.academic_course_id ?? ""}
                        onChange={(e) =>
                          choose(
                            `course:${c.preview_course_id}`,
                            e.target.value
                              ? {
                                  kind: "LINK_COURSE",
                                  preview_course_id: c.preview_course_id,
                                  academic_course_id: e.target.value,
                                }
                              : null,
                          )
                        }
                      >
                        <option value="">按新课程处理</option>
                        {timetable?.courses.map((old) => (
                          <option key={old.id} value={old.id}>
                            {old.name}
                          </option>
                        ))}
                      </select>
                    </label>
                  ))}
                {preview.courses.flatMap((c) =>
                  c.sessions
                    .filter(
                      (s) =>
                        diff?.ambiguous?.some(
                          (a) => a.preview_session_id === s.preview_session_id,
                        ) || diff?.added?.includes(s.preview_session_id),
                    )
                    .map((s) => {
                      const courseID =
                        choices[`course:${c.preview_course_id}`]?.academic_course_id ??
                        c.academic_course_id
                      const candidates =
                        timetable?.courses.find((old) => old.id === courseID)?.sessions ?? []
                      const ambiguous = diff?.ambiguous?.some(
                        (a) => a.preview_session_id === s.preview_session_id,
                      )
                      if (!ambiguous && candidates.length === 0) return null
                      const key = `session:${s.preview_session_id}`
                      return (
                        <label className="xq-henu-choice" key={key}>
                          {c.name} · {weekdays[s.weekday - 1]} · {s.sections.join("、")} 节
                          <select
                            value={
                              choices[key]?.kind === "CREATE_SESSION"
                                ? "NEW"
                                : (choices[key]?.session_id ?? "")
                            }
                            onChange={(e) =>
                              choose(
                                key,
                                !e.target.value
                                  ? null
                                  : e.target.value === "NEW"
                                    ? {
                                        kind: "CREATE_SESSION",
                                        preview_session_id: s.preview_session_id,
                                      }
                                    : {
                                        kind: "LINK_SESSION",
                                        preview_session_id: s.preview_session_id,
                                        session_id: e.target.value,
                                      },
                              )
                            }
                          >
                            <option value="">
                              {ambiguous ? "请选择如何处理" : "按新增安排处理"}
                            </option>
                            <option value="NEW">确认为新增安排</option>
                            {candidates.map((old) => (
                              <option key={old.session_id} value={old.session_id}>
                                关联：{sessionLabel(old.session_id)}
                              </option>
                            ))}
                          </select>
                        </label>
                      )
                    }),
                )}
                {diff?.missing?.map((m) => (
                  <label className="xq-henu-choice" key={m.session_id}>
                    {sessionLabel(m.session_id)} · 本次文件未出现
                    <select
                      value={
                        choices[`missing:${m.session_id}`]?.kind ??
                        (m.decision ? `${m.decision}_SESSION` : "")
                      }
                      onChange={(e) =>
                        choose(
                          `missing:${m.session_id}`,
                          e.target.value
                            ? {
                                kind: e.target.value as "RETAIN_SESSION" | "RETIRE_SESSION",
                                session_id: m.session_id,
                              }
                            : null,
                        )
                      }
                    >
                      <option value="">请选择</option>
                      <option value="RETAIN_SESSION">保留已有安排</option>
                      <option value="RETIRE_SESSION">停用已有安排</option>
                    </select>
                  </label>
                ))}
                <p>选择只用于生成新预览；保存后再次核对，才会正式导入。</p>
                <button
                  className="xq-button"
                  disabled={
                    busy ||
                    dirty ||
                    expired ||
                    revision === undefined ||
                    (!pendingChoices && !stale)
                  }
                  onClick={rebuild}
                >
                  保存选择并更新预览
                </button>
              </fieldset>
            </details>
          )}
          <fieldset className="xq-henu-approval" disabled={frozen}>
            {warnings.map((code) => (
              <label className="xq-henu-confirm" key={code}>
                <input
                  type="checkbox"
                  checked={ack.includes(code)}
                  onChange={(e) =>
                    setAck((old) =>
                      e.target.checked ? [...old, code] : old.filter((c) => c !== code),
                    )
                  }
                />
                {code === "MISSING_LOCATION"
                  ? "我知道部分安排没有教室，仍要导入"
                  : (preview.issues.find((i) => i.code === code)?.message ?? code)}
              </label>
            ))}
            <label className="xq-henu-confirm">
              <input
                type="checkbox"
                checked={reviewed}
                onChange={(e) => setReviewed(e.target.checked)}
              />
              我已核对目标学期、课程、周次和时间
            </label>
          </fieldset>
          {stale && <p className="xq-henu-message">课表或设置已变化，请先更新预览并重新核对。</p>}
          {expired && (
            <p className="xq-henu-message">
              预览已过期，请
              {preview.acquisition === "DIRECT_FETCH" ? "关闭并重新检查更新" : "重新上传文件"}。
            </p>
          )}
          <button
            className="xq-button primary"
            disabled={busy || (!uncertain && (!canSubmit || !diff))}
            onClick={submit}
          >
            {busy ? "正在处理…" : uncertain ? "使用原请求重试" : "确认导入课表"}
          </button>
          {!preview.can_commit && <p>请先处理上方待核对内容；配置缺失时需先补齐学期设置。</p>}
        </>
      )}
    </section>
  )
}
