import { useState, useEffect } from "react"
import type { Exam, CreateExamRequest, UpdateExamRequest } from "@/types/exam"
import type { ImportTextResult } from "@/types/course-import"
import {
  listExams,
  createExam,
  updateExam,
  deleteExam,
  importExamsFromText,
} from "@/lib/api/exams"
import { Button } from "@/components/ui/button"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { Modal } from "@/components/shared/modal"
import { cn } from "@/lib/utils"
import { userErrorMessage } from "@/lib/errors"
import { formatSemester } from "@/lib/semester"
import { ArrowLeft, Plus, Trash2, Pencil, Save, Clock, MapPin, FileText, ClipboardPaste, Upload } from "lucide-react"

const FIELD_CLASS =
  "w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"

// 导入入口暂时隐藏：现有解析器面向本科教务 TSV，与研究生系统不兼容；该功能后续可能弃用。恢复/适配后改回 true 即可。
const ENABLE_EXAM_IMPORT = false

function computeDaysUntil(datetimeStr: string): number {
  const target = new Date(datetimeStr)
  const now = new Date()
  const diffMs = target.getTime() - now.getTime()
  return Math.ceil(diffMs / (1000 * 60 * 60 * 24))
}

function daysBadgeClass(days: number): string {
  if (days < 0) return "bg-muted text-muted-foreground"
  if (days <= 7) return "bg-red-500/15 text-red-400 border-red-500/30"
  if (days <= 14) return "bg-amber-500/15 text-amber-400 border-amber-500/30"
  return "bg-emerald-500/15 text-emerald-400 border-emerald-500/30"
}

function daysLabel(days: number): string {
  if (days < 0) return `已过 ${Math.abs(days)} 天`
  if (days === 0) return "今天"
  if (days === 1) return "明天"
  return `剩余 ${days} 天`
}

function formatDate(datetimeStr: string): string {
  const d = new Date(datetimeStr)
  if (Number.isNaN(d.getTime())) return ""
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  const hour = String(d.getHours()).padStart(2, "0")
  const min = String(d.getMinutes()).padStart(2, "0")
  return `${year}-${month}-${day} ${hour}:${min}`
}

function formatTimeOnly(datetimeStr: string): string {
  const d = new Date(datetimeStr)
  if (Number.isNaN(d.getTime())) return ""
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`
}

interface ExamFormData {
  course_name: string
  exam_datetime: string
  exam_end_datetime: string
  location: string
  notes: string
  semester: string
  course_id: number | null
}

function emptyForm(): ExamFormData {
  return {
    course_name: "",
    exam_datetime: "",
    exam_end_datetime: "",
    location: "",
    notes: "",
    semester: "",
    course_id: null,
  }
}

function examToForm(e: Exam): ExamFormData {
  return {
    course_name: e.course_name,
    exam_datetime: e.exam_datetime,
    exam_end_datetime: e.exam_end_datetime,
    location: e.location,
    notes: e.notes,
    semester: e.semester,
    course_id: e.course_id,
  }
}

function formatDatetimeForInput(isoStr: string): string {
  const d = new Date(isoStr)
  if (Number.isNaN(d.getTime())) return ""
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  const hour = String(d.getHours()).padStart(2, "0")
  const min = String(d.getMinutes()).padStart(2, "0")
  return `${year}-${month}-${day}T${hour}:${min}`
}

export function ExamList({ onNavigate }: { onNavigate: (key: string) => void }) {
  const [exams, setExams] = useState<Exam[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [editingExam, setEditingExam] = useState<Exam | null>(null)
  const [form, setForm] = useState<ExamFormData>(emptyForm())
  const [saving, setSaving] = useState(false)

  const [showImport, setShowImport] = useState(false)
  const [importText, setImportText] = useState("")
  const [importSemester, setImportSemester] = useState("")
  const [importing, setImporting] = useState(false)
  const [importError, setImportError] = useState<string | null>(null)
  const [importFeedback, setImportFeedback] = useState<ImportTextResult | null>(null)

  async function fetchExams() {
    const result = await listExams()
    const enriched = result.map((e) => ({ ...e, days_until: computeDaysUntil(e.exam_datetime) }))
    setExams(enriched)
    setError(null)
  }

  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const result = await listExams()
        const enriched = result.map((e) => ({ ...e, days_until: computeDaysUntil(e.exam_datetime) }))
        if (!cancelled) {
          setExams(enriched)
          setError(null)
        }
      } catch {
        if (!cancelled) setError("Tauri 不可用 — 展示离线 UI")
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [])

  function openCreateForm() {
    setForm(emptyForm())
    setEditingExam(null)
    setShowForm(true)
  }

  function openEditForm(exam: Exam) {
    setForm(examToForm(exam))
    setEditingExam(exam)
    setShowForm(true)
  }

  function openImportPanel() {
    setImportError(null)
    setImportFeedback(null)
    setImportText("")
    setShowImport(true)
  }

  async function handleSave() {
    if (!form.course_name.trim() || !form.exam_datetime) return

    setSaving(true)
    try {
      if (editingExam) {
        const payload: UpdateExamRequest = {
          course_name: form.course_name,
          exam_datetime: form.exam_datetime,
          exam_end_datetime: form.exam_end_datetime,
          location: form.location,
          notes: form.notes,
          semester: form.semester,
          course_id: form.course_id,
        }
        await updateExam(editingExam.id, payload)
      } else {
        const payload: CreateExamRequest = {
          course_name: form.course_name,
          exam_datetime: form.exam_datetime,
          exam_end_datetime: form.exam_end_datetime || undefined,
          location: form.location || undefined,
          notes: form.notes || undefined,
          semester: form.semester || undefined,
          course_id: form.course_id || undefined,
        }
        await createExam(payload)
      }
      setShowForm(false)
      setEditingExam(null)
      await fetchExams()
      setActionError(null)
    } catch (e) {
      setActionError(userErrorMessage(e, "保存考试失败"))
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    try {
      await deleteExam(id)
      await fetchExams()
      setActionError(null)
    } catch (e) {
      setActionError(userErrorMessage(e, "删除考试失败"))
    }
  }

  async function handleReadClipboard() {
    setImportError(null)
    try {
      await new Promise((r) => setTimeout(r, 150))
      const text = await navigator.clipboard.readText()
      if (!text.trim()) {
        setImportError("剪贴板为空，请先从教务系统复制考试安排表格。")
        return
      }
      setImportText(text)
    } catch {
      setImportError("读取剪贴板失败，请手动粘贴考试文本后再导入。")
    }
  }

  async function handleImport() {
    if (!importText.trim()) {
      setImportError("请先粘贴或读取考试文本。")
      return
    }

    setImporting(true)
    setImportError(null)
    setImportFeedback(null)
    try {
      const result = await importExamsFromText({
        text: importText,
        semester: importSemester.trim(),
      })
      setImportFeedback(result)
      await fetchExams()
    } catch (e) {
      setImportError(typeof e === "string" ? e : "导入失败，请检查考试文本格式。")
    } finally {
      setImporting(false)
    }
  }

  const upcomingExams = exams.filter((e) => (e.days_until ?? 0) >= 0)
  const pastExams = exams.filter((e) => (e.days_until ?? 0) < 0)

  return (
    <div className="w-full max-w-2xl mx-auto space-y-4 animate-in fade-in-0 slide-in-from-bottom-2 duration-300">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-1">
          <Button size="icon-sm" variant="ghost" onClick={() => onNavigate("kairos")} aria-label="返回">
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <h2 className="text-lg font-heading font-medium text-foreground">考试倒计时</h2>
        </div>
        <div className="flex items-center gap-2">
          {ENABLE_EXAM_IMPORT && (
            <Button size="sm" variant="outline" onClick={openImportPanel} disabled={importing} className="px-2.5 sm:px-3">
              <ClipboardPaste className="h-4 w-4 sm:mr-1" />
              <span className="hidden sm:inline">从剪贴板导入</span>
            </Button>
          )}
          <Button size="sm" onClick={openCreateForm} className="px-2.5 sm:px-3">
            <Plus className="h-4 w-4 sm:mr-1" />
            <span className="hidden sm:inline">添加考试</span>
          </Button>
        </div>
      </div>

      {actionError && (
        <p className="text-center text-sm text-destructive">{actionError}</p>
      )}

      {loading && (
        <div className="flex items-center justify-center py-12">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      )}

      {!loading && error && exams.length === 0 && (
        <p className="py-8 text-center text-sm text-muted-foreground">{error}</p>
      )}

      {!loading && exams.length === 0 && !error && (
        <AcrylicPanel className="p-8 text-center">
          <p className="text-sm text-muted-foreground">暂无考试</p>
          <p className="mt-1 text-xs text-muted-foreground/60">点击"添加考试"按钮添加第一个考试</p>
        </AcrylicPanel>
      )}

      {!loading && upcomingExams.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">即将到来</h3>
          {upcomingExams.map((exam) => (
            <ExamCard key={exam.id} exam={exam} onEdit={() => openEditForm(exam)} onDelete={() => void handleDelete(exam.id)} />
          ))}
        </div>
      )}

      {!loading && pastExams.length > 0 && (
        <div className="space-y-2">
          <h3 className="mt-6 text-xs font-medium uppercase tracking-wider text-muted-foreground">已结束</h3>
          {pastExams.map((exam) => (
            <ExamCard key={exam.id} exam={exam} onEdit={() => openEditForm(exam)} onDelete={() => void handleDelete(exam.id)} />
          ))}
        </div>
      )}

      {/* 考试编辑/新建模态 */}
      <Modal
        open={showForm}
        onOpenChange={(open) => {
          setShowForm(open)
          if (!open) setEditingExam(null)
        }}
        title={editingExam ? "编辑考试" : "新建考试"}
        description="填写考试名称、时间与地点等信息"
        className="max-w-xl"
      >
        <div className="grid grid-cols-2 gap-3">
          <div className="col-span-2">
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              考试名称 <span className="text-destructive">*</span>
            </label>
            <input
              type="text"
              value={form.course_name}
              onChange={(e) => setForm({ ...form, course_name: e.target.value })}
              required
              placeholder="如：高数期末考试"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              开始时间 <span className="text-destructive">*</span>
            </label>
            <input
              type="datetime-local"
              value={form.exam_datetime ? formatDatetimeForInput(form.exam_datetime) : ""}
              onChange={(e) =>
                setForm({ ...form, exam_datetime: e.target.value ? new Date(e.target.value).toISOString() : "" })
              }
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">结束时间</label>
            <input
              type="datetime-local"
              value={form.exam_end_datetime ? formatDatetimeForInput(form.exam_end_datetime) : ""}
              onChange={(e) =>
                setForm({ ...form, exam_end_datetime: e.target.value ? new Date(e.target.value).toISOString() : "" })
              }
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">地点</label>
            <input
              type="text"
              value={form.location}
              onChange={(e) => setForm({ ...form, location: e.target.value })}
              placeholder="考场/教室"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">学期</label>
            <input
              type="text"
              value={form.semester}
              onChange={(e) => setForm({ ...form, semester: e.target.value })}
              placeholder="如：2026S1"
              className={FIELD_CLASS}
            />
          </div>
          <div className="col-span-2">
            <label className="mb-1 block text-xs font-medium text-muted-foreground">备注</label>
            <textarea
              value={form.notes}
              onChange={(e) => setForm({ ...form, notes: e.target.value })}
              placeholder="考试注意事项（可选）"
              rows={2}
              className="w-full resize-none rounded-md border border-input bg-background px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
            />
          </div>
        </div>

        <div className="mt-5 flex items-center justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              setShowForm(false)
              setEditingExam(null)
            }}
          >
            取消
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={saving || !form.course_name.trim() || !form.exam_datetime}
            onClick={handleSave}
          >
            <Save className="mr-1 h-3.5 w-3.5" />
            {saving ? "保存中..." : "保存"}
          </Button>
        </div>
      </Modal>

      {ENABLE_EXAM_IMPORT && (
        <Modal
          open={showImport}
          onOpenChange={(open) => {
            setShowImport(open)
            if (!open) {
              setImportError(null)
              setImportFeedback(null)
            }
          }}
          title="从剪贴板导入考试"
          description="从教务系统网页复制考试安排表格后粘贴或读取剪贴板"
          className="max-w-2xl"
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_200px]">
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                考试文本 <span className="text-destructive">*</span>
              </label>
              <textarea
                value={importText}
                onChange={(e) => setImportText(e.target.value)}
                placeholder="从教务系统复制考试安排表格后粘贴到这里。"
                rows={8}
                className="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
              />
            </div>
            <div className="space-y-3">
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">学期</label>
                <input
                  type="text"
                  value={importSemester}
                  onChange={(e) => setImportSemester(e.target.value)}
                  placeholder="如：2026S1"
                  className={FIELD_CLASS}
                />
              </div>
              <div className="rounded-md border border-border/60 bg-background/60 p-3 text-xs text-muted-foreground space-y-1">
                <p>解析与去重在后端完成。</p>
                <p>重复考试会被自动跳过。</p>
              </div>
            </div>
          </div>

          {importError && <p className="mt-3 text-sm text-destructive">{importError}</p>}
          {importFeedback && <p className="mt-3 text-sm text-muted-foreground">{importFeedback.message}</p>}

          <div className="mt-4 flex items-center justify-end gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={() => void handleReadClipboard()}>
              <ClipboardPaste className="mr-1 h-3.5 w-3.5" />
              读取剪贴板
            </Button>
            <Button type="button" size="sm" disabled={importing || !importText.trim()} onClick={() => void handleImport()}>
              <Upload className="mr-1 h-3.5 w-3.5" />
              {importing ? "导入中..." : "开始导入"}
            </Button>
          </div>
        </Modal>
      )}
    </div>
  )
}

function ExamCard({ exam, onEdit, onDelete }: { exam: Exam; onEdit: () => void; onDelete: () => void }) {
  const days = exam.days_until ?? 0
  const isPast = days < 0
  const endTime = exam.exam_end_datetime ? formatTimeOnly(exam.exam_end_datetime) : ""

  return (
    <AcrylicPanel
      className={cn(
        "bg-card p-3 transition-all hover:-translate-y-0.5 hover:bg-card/95 hover:shadow-md",
        isPast && "opacity-50",
      )}
    >
      <div className="flex items-start gap-3">
        <div
          className={cn(
            "inline-flex shrink-0 items-center rounded-md border px-2 py-1 text-xs font-semibold tabular-nums",
            daysBadgeClass(days),
          )}
        >
          {daysLabel(days)}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium text-foreground">{exam.course_name}</span>
            {exam.semester && <span className="shrink-0 text-[10px] text-muted-foreground/50">{formatSemester(exam.semester)}</span>}
          </div>

          <div className="mt-1 flex items-center gap-3">
            <span className="inline-flex items-center gap-1 text-[11px] text-muted-foreground">
              <Clock className="h-3 w-3" />
              {formatDate(exam.exam_datetime)}
              {endTime && `–${endTime}`}
            </span>
            {exam.location && (
              <span className="inline-flex items-center gap-1 text-[11px] text-muted-foreground">
                <MapPin className="h-3 w-3" />
                {exam.location}
              </span>
            )}
          </div>

          {exam.notes && (
            <div className="mt-1 flex items-start gap-1">
              <FileText className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground/50" />
              <p className="text-xs text-muted-foreground/70">{exam.notes}</p>
            </div>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-1">
          <Button variant="ghost" size="icon" className="h-9 w-9 sm:h-7 sm:w-7 text-muted-foreground hover:text-foreground" onClick={onEdit}>
            <Pencil className="h-4 w-4 sm:h-3.5 sm:w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-9 w-9 sm:h-7 sm:w-7 text-muted-foreground hover:text-destructive"
            onClick={onDelete}
          >
            <Trash2 className="h-4 w-4 sm:h-3.5 sm:w-3.5" />
          </Button>
        </div>
      </div>
    </AcrylicPanel>
  )
}
