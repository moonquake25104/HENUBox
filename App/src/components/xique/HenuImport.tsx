import { useEffect, useState, type FormEvent } from "react"
import { Upload, FileSpreadsheet, CheckCircle2, AlertCircle, Settings2 } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import {
  getAcademicSettings,
  saveAcademicSettings,
  uploadAcademicPreview,
  getAcademicPreview,
  type SemesterSettings,
  type AcademicPreview,
} from "@/lib/api/academic"
import { userErrorMessage } from "@/lib/errors"
import { semesterLabel, weekdays } from "./dates"
import { HenuConfirmation } from "./HenuConfirmation"

// User-provided Sheet1!A2:B14, 2026-10-23. Applied only after an explicit click.
const suppliedTimes = [
  "08:30-09:15",
  "09:25-10:10",
  "10:20-11:05",
  "11:15-12:00",
  "12:00-12:45",
  "13:30-14:15",
  "14:25-15:10",
  "15:20-16:05",
  "16:15-17:00",
  "17:00-17:45",
  "18:10-18:55",
  "19:05-19:50",
  "19:55-20:40",
]
const statusNames: Record<string, string> = {
  READY: "解析完整",
  INVALID: "有内容无法识别",
  NEEDS_REVIEW: "需要核对",
  NEEDS_CONFIGURATION: "请补充学期设置",
  COMMITTED: "已提交",
}
function ranges(values: number[]) {
  const result: string[] = []
  for (let i = 0; i < values.length; i++) {
    const first = values[i]
    while (i + 1 < values.length && values[i + 1] === values[i] + 1) i++
    result.push(first === values[i] ? String(first) : `${first}–${values[i]}`)
  }
  return result.join("、")
}
export function HenuImport({
  semester,
  close,
  onCommitted,
  initialPreviewId,
}: {
  semester: string
  close: () => void
  onCommitted: () => Promise<void>
  initialPreviewId?: string
}) {
  const [settings, setSettings] = useState<SemesterSettings | null>(null)
  const [preview, setPreview] = useState<AcademicPreview | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const [dirty, setDirty] = useState(false)
  const [retry, setRetry] = useState(0)
  const direct = Boolean(initialPreviewId)
  useEffect(() => {
    let live = true
    Promise.all([
      getAcademicSettings(semester),
      initialPreviewId ? getAcademicPreview(initialPreviewId) : Promise.resolve(null),
    ])
      .then(([value, existing]) => {
        if (live) {
          setSettings(value)
          setPreview(existing)
          setDirty(false)
        }
      })
      .catch((err) => {
        if (live)
          setError(
            userErrorMessage(
              err,
              direct ? "预览无法读取，请返回连接窗口重新检查更新" : "读取学期设置失败",
            ),
          )
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [semester, retry, initialPreviewId, direct])
  function update(value: SemesterSettings) {
    setSettings(value)
    setDirty(true)
    setNotice("")
  }
  function fillSupplied() {
    if (!settings) return
    update({
      ...settings,
      semester_label: "2026-2027学年第一学期",
      week_anchor: { start_date: "2026-08-31", source: "USER", confirmed: false },
      periods: suppliedTimes.map((time, i) => ({
        index: i + 1,
        start_time: time.split("-")[0],
        end_time: time.split("-")[1],
        configured: true,
      })),
    })
    setNotice("已填入你提供的学期与作息。请核对目标学期后保存。")
  }
  async function save(event: FormEvent) {
    event.preventDefault()
    if (!settings || busy) return
    if (
      settings.periods.some(
        (p) =>
          Boolean(p.start_time) !== Boolean(p.end_time) ||
          (p.start_time && p.end_time && p.start_time >= p.end_time),
      )
    ) {
      setError("请检查每节课的开始和结束时间")
      return
    }
    setBusy(true)
    setError("")
    setNotice("")
    try {
      const saved = await saveAcademicSettings(settings)
      setSettings(saved)
      setDirty(false)
      if (preview)
        setPreview({
          ...preview,
          can_commit: false,
          validation: {
            state: "STALE",
            current_aggregate_revision: preview.validation?.current_aggregate_revision ?? 0,
          },
        })
      setNotice(
        direct
          ? "学期设置已保存。请关闭预览，重新检查更新后核对。"
          : "学期设置已保存。请重新上传文件，按新作息生成预览。",
      )
    } catch (err) {
      setError(
        `${userErrorMessage(err, "保存失败")}。若配置已被修改，请关闭后重新打开，核对最新设置。`,
      )
    } finally {
      setBusy(false)
    }
  }
  async function upload() {
    if (!file || busy || dirty) return
    if (file.size > 8 * 1024 * 1024) {
      setError("文件不能超过 8 MiB")
      return
    }
    setBusy(true)
    setError("")
    setNotice("")
    // Remove the old result so a failed new upload is never mistaken for success.
    setPreview(null)
    try {
      setPreview(await uploadAcademicPreview(semester, file))
    } catch (err) {
      setError(userErrorMessage(err, "解析失败，请检查导出文件后重试"))
    } finally {
      setBusy(false)
    }
  }
  async function refresh() {
    if (!preview || busy) return
    setBusy(true)
    setError("")
    try {
      setPreview(await getAcademicPreview(preview.preview_id))
    } catch (err) {
      setPreview(null)
      setError(userErrorMessage(err, "预览无法读取，请重新上传"))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open
      onOpenChange={(open) => {
        if (!open && !busy) close()
      }}
      title="从河大课表开始"
      description={
        direct
          ? "已从学校获取课表，核对变化后再确认更新。"
          : "上传教务导出文件，核对每一门课，再确认加入课表。"
      }
      className="xq-henu-modal"
    >
      <div className="xq-henu-target">
        <FileSpreadsheet size={21} />
        <div>
          <strong>目标学期 · {semesterLabel(semester)}</strong>
          <small>
            {direct ? "来自学校本次检查" : "教务导出文件导入"} · {semester}
          </small>
          {preview?.source_obtained_at && (
            <small>
              获取于{" "}
              {new Date(preview.source_obtained_at).toLocaleString("zh-CN", {
                timeZone: "Asia/Shanghai",
              })}
              （北京时间）
            </small>
          )}
        </div>
      </div>
      {error && (
        <div className="xq-henu-message error" role="alert">
          <AlertCircle size={18} />
          <span>{error}</span>
        </div>
      )}
      {notice && (
        <p className="xq-henu-message" role="status">
          {notice}
        </p>
      )}
      {loading ? (
        <p role="status">正在读取学期设置…</p>
      ) : !settings ? (
        <button
          className="xq-button"
          onClick={() => {
            setError("")
            setLoading(true)
            setRetry(retry + 1)
          }}
        >
          重新读取设置
        </button>
      ) : (
        <>
          <details className="xq-henu-settings" open={dirty || undefined}>
            <summary>
              <Settings2 size={17} />
              学期与 13 节作息{" "}
              <span>{settings.periods.filter((p) => p.configured).length}/13 已配置</span>
            </summary>
            <form onSubmit={save}>
              <fieldset disabled={busy}>
                <button type="button" className="xq-button small" onClick={fillSupplied}>
                  填入已提供的学期与作息
                </button>
                <div className="xq-henu-fields">
                  <label>
                    学校学期名称
                    <input
                      maxLength={128}
                      value={settings.semester_label ?? ""}
                      placeholder="例如 2026-2027学年第一学期"
                      onChange={(e) =>
                        update({ ...settings, semester_label: e.target.value || null })
                      }
                    />
                  </label>
                  <label>
                    第一教学周周一
                    <input
                      type="date"
                      value={settings.week_anchor.start_date ?? ""}
                      onChange={(e) =>
                        update({
                          ...settings,
                          week_anchor: {
                            start_date: e.target.value || null,
                            source: e.target.value ? "USER" : "UNKNOWN",
                            confirmed: false,
                          },
                        })
                      }
                    />
                  </label>
                </div>
                <label className="xq-henu-confirm">
                  <input
                    type="checkbox"
                    disabled={!settings.week_anchor.start_date}
                    checked={settings.week_anchor.confirmed}
                    onChange={(e) =>
                      update({
                        ...settings,
                        week_anchor: { ...settings.week_anchor, confirmed: e.target.checked },
                      })
                    }
                  />
                  确认此日期和作息适用于当前目标学期
                </label>
                <div className="xq-periods">
                  {settings.periods.map((p, i) => (
                    <div key={p.index}>
                      <span>第 {p.index} 节</span>
                      {(["start_time", "end_time"] as const).map((key, j) => (
                        <input
                          key={key}
                          type="time"
                          aria-label={`第${p.index}节${j ? "结束" : "开始"}时间`}
                          value={p[key] ?? ""}
                          onChange={(e) => {
                            const periods = settings.periods
                              .map((v, n) =>
                                n === i ? { ...v, [key]: e.target.value || null } : v,
                              )
                              .map((v) => ({
                                ...v,
                                configured: Boolean(v.start_time && v.end_time),
                              }))
                            update({ ...settings, periods })
                          }}
                        />
                      ))}
                    </div>
                  ))}
                </div>
                <button
                  className="xq-button primary"
                  disabled={
                    !dirty ||
                    busy ||
                    Boolean(settings.week_anchor.start_date && !settings.week_anchor.confirmed)
                  }
                >
                  保存学期设置
                </button>
              </fieldset>
            </form>
          </details>
          {!direct && (
            <section className="xq-henu-upload">
              <label htmlFor="henu-file">
                <Upload size={24} />
                <strong>选择教务导出文件</strong>
                <span>HTML 型 .xls / .html · 最大 8 MiB</span>
              </label>
              <input
                id="henu-file"
                type="file"
                accept=".xls,.html,.htm"
                disabled={busy}
                onChange={(e) => {
                  setFile(e.target.files?.[0] ?? null)
                  setPreview(null)
                  setError("")
                }}
              />
              <button
                className="xq-button primary"
                disabled={!file || busy || dirty}
                onClick={upload}
              >
                {busy ? "正在处理…" : "解析并预览"}
              </button>
              {dirty && <p>请先保存学期设置，再生成预览。</p>}
            </section>
          )}
        </>
      )}
      {preview && preview.status !== "COMMITTED" && (
        <section className="xq-henu-preview" aria-label="课表解析结果">
          <div className="xq-henu-result">
            {preview.status === "READY" && preview.validation?.state === "FRESH" ? (
              <CheckCircle2 size={23} />
            ) : (
              <AlertCircle size={23} />
            )}
            <div>
              <h3>
                {preview.validation?.state === "STALE"
                  ? "课表或设置已变化，请更新预览"
                  : (statusNames[preview.status] ?? "请核对预览")}
              </h3>
              <p>{preview.source_semester_label ?? "学校学期尚待确认"}</p>
            </div>
          </div>
          <div className="xq-henu-counts">
            <strong>
              {preview.summary.logical_courses}
              <small>门课程</small>
            </strong>
            <strong>
              {preview.summary.sessions}
              <small>个安排</small>
            </strong>
            <strong>
              {preview.summary.unparsed_fragments}
              <small>未识别片段</small>
            </strong>
          </div>
          {preview.issues.length > 0 && (
            <details className="xq-henu-issues">
              <summary>查看 {preview.issues.length} 项提示与待处理内容</summary>
              <ul>
                {preview.issues.map((issue, i) => (
                  <li key={i}>
                    <b>{issue.severity === "BLOCKING" ? "待处理" : "提示"}</b> {issue.message}
                    {issue.preview_course_id && (
                      <small>
                        {" "}
                        ·{" "}
                        {
                          preview.courses.find(
                            (c) => c.preview_course_id === issue.preview_course_id,
                          )?.name
                        }
                      </small>
                    )}
                  </li>
                ))}
              </ul>
            </details>
          )}
          <div className="xq-henu-courses">
            {preview.courses.map((course) => (
              <article key={course.preview_course_id}>
                <h4>
                  {course.name}
                  {course.credits && <small>{course.credits} 学分</small>}
                </h4>
                {course.sessions.map((s) => (
                  <div key={s.preview_session_id} className="xq-henu-session">
                    <strong>
                      {weekdays[s.weekday - 1]} · 第 {ranges(s.sections)} 节
                    </strong>
                    <span>
                      {s.start_time && s.end_time ? `${s.start_time}–${s.end_time}` : "时刻待补充"}{" "}
                      · 第 {ranges(s.weeks)} 周
                    </span>
                    <span>
                      {s.location || "教室待定"}
                      {s.teacher ? ` · ${s.teacher}` : ""}
                    </span>
                  </div>
                ))}
              </article>
            ))}
          </div>
          <div className="xq-henu-footer">
            <button className="xq-button small" disabled={busy} onClick={refresh}>
              检查预览状态
            </button>
            <p>当前为预览，请在下方核对变化并确认导入。</p>
            <small>
              预览有效至{" "}
              {new Date(preview.expires_at).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" })}
              （北京时间）
            </small>
          </div>
        </section>
      )}
      {preview && (
        <HenuConfirmation
          preview={preview}
          semester={semester}
          busy={busy}
          dirty={dirty}
          setBusy={setBusy}
          onPreview={setPreview}
          onCommitted={onCommitted}
          onDone={close}
        />
      )}
    </Modal>
  )
}
