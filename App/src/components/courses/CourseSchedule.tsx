import { useEffect, useRef, useState } from "react"
import type { Course, CreateCourseRequest, UpdateCourseRequest, CourseFilterParams } from "@/types/course"
import {
  listCourses,
  getWeekSchedule,
  createCourse,
  updateCourse,
  deleteCourse,
  importCoursesFromText,
  resetSemesterDates,
} from "@/lib/api/courses"
import type { ImportTextResult } from "@/types/course-import"
import type { WeekScheduleItem, WeekScheduleResponse } from "@/types/schedule"
import { Button } from "@/components/ui/button"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { Modal } from "@/components/shared/modal"
import { cn } from "@/lib/utils"
import { userErrorMessage } from "@/lib/errors"
import { formatSemester } from "@/lib/semester"
import {
  Plus,
  Pencil,
  Trash2,
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  EllipsisVertical,
  Save,
} from "lucide-react"
import {
  DAY_LABELS,
  DAY_SHORT,
  type CourseFormData,
  emptyForm,
  courseToForm,
  todayKey,
  computeCurrentWeek,
  dayCellKey,
  FIELD_CLASS,
} from "./utils"
import { WeekGrid } from "./WeekGrid"
import { CourseFormModal } from "./CourseFormModal"
import { ImportModal } from "./ImportModal"
import { CourseDetailModal } from "./CourseDetailModal"

// 导入入口暂时隐藏：现有解析器面向本科教务 TSV，与研究生系统不兼容；该功能后续可能弃用。恢复/适配后改回 true 即可。
const ENABLE_COURSE_IMPORT = false

const PHASE_LABELS: Record<WeekScheduleResponse["phase_type"], string> = {
  unknown: "未识别阶段",
  teaching: "教学周",
  exam: "考试周",
  break: "假期",
}

export function CourseSchedule({ onNavigate }: { onNavigate: (key: string) => void }) {
  const [courses, setCourses] = useState<Course[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [semesterFilter, setSemesterFilter] = useState(() => {
    const now = new Date()
    const y = now.getFullYear()
    // Feb–Jul → S1 (春季), Aug–Jan → S2 (秋季)
    return now.getMonth() >= 1 && now.getMonth() <= 6 ? `${y}S1` : `${y}S2`
  })

  const [viewMode, setViewMode] = useState<"week" | "list">("week")
  const [weekIndex, setWeekIndex] = useState(1)
  const [weekData, setWeekData] = useState<WeekScheduleResponse | null>(null)
  const [weekLoading, setWeekLoading] = useState(true)
  const [weekError, setWeekError] = useState<string | null>(null)
  const [weekRefresh, setWeekRefresh] = useState(0)
  const [mobileDayIndex, setMobileDayIndex] = useState(new Date().getDay() === 0 ? 6 : new Date().getDay() - 1)
  const didAutoJumpRef = useRef(false)

  const [showForm, setShowForm] = useState(false)
  const [editingCourse, setEditingCourse] = useState<Course | null>(null)
  const [form, setForm] = useState<CourseFormData>(emptyForm())
  const [saving, setSaving] = useState(false)

  const [showImport, setShowImport] = useState(false)
  const [importText, setImportText] = useState("")
  const [importSemester, setImportSemester] = useState("")
  const [importStartDate, setImportStartDate] = useState("")
  const [importing, setImporting] = useState(false)
  const [importError, setImportError] = useState<string | null>(null)
  const [importFeedback, setImportFeedback] = useState<ImportTextResult | null>(null)

  const [showWeekPicker, setShowWeekPicker] = useState(false)
  const [showOverflowMenu, setShowOverflowMenu] = useState(false)
  const [showResetDate, setShowResetDate] = useState(false)
  const [resetDate, setResetDate] = useState("")
  const [resetting, setResetting] = useState(false)
  const [showDetail, setShowDetail] = useState(false)
  const [detailCourse, setDetailCourse] = useState<Course | null>(null)
  const weekPickerRef = useRef<HTMLDivElement>(null)
  const overflowMenuRef = useRef<HTMLDivElement>(null)

  // Close week picker on outside click
  useEffect(() => {
    if (!showWeekPicker) return
    function handleClick(e: MouseEvent) {
      if (weekPickerRef.current && !weekPickerRef.current.contains(e.target as Node)) {
        setShowWeekPicker(false)
      }
    }
    document.addEventListener("mousedown", handleClick)
    return () => document.removeEventListener("mousedown", handleClick)
  }, [showWeekPicker])

  // Close overflow menu on outside click
  useEffect(() => {
    if (!showOverflowMenu) return
    function handleClick(e: MouseEvent) {
      if (overflowMenuRef.current && !overflowMenuRef.current.contains(e.target as Node)) {
        setShowOverflowMenu(false)
      }
    }
    document.addEventListener("mousedown", handleClick)
    return () => document.removeEventListener("mousedown", handleClick)
  }, [showOverflowMenu])

  async function getCourses(targetSemester: string): Promise<Course[]> {
    const filters: CourseFilterParams = {}
    if (targetSemester) filters.semester = targetSemester
    return listCourses(filters)
  }

  async function fetchCourses(targetSemester = semesterFilter) {
    const result = await getCourses(targetSemester)
    setCourses(result)
    setError(null)
  }

  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const result = await getCourses(semesterFilter)
        if (!cancelled) {
          setCourses(result)
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
  }, [semesterFilter])

  // 切换学期后允许重新自动跳转到当前周
  useEffect(() => {
    didAutoJumpRef.current = false
  }, [semesterFilter])

  // 周视图数据
  useEffect(() => {
    if (viewMode !== "week") return
    let cancelled = false
    async function loadWeek() {
      setWeekLoading(true)
      setWeekError(null)
      try {
        const res = await getWeekSchedule({
          semester: semesterFilter,
          week_index: weekIndex,
        })
        if (cancelled) return
        setWeekData(res)
        if (!didAutoJumpRef.current && res.semester_start_date) {
          didAutoJumpRef.current = true
          const current = computeCurrentWeek(res.semester_start_date)
          if (current !== weekIndex) setWeekIndex(current)
        }
      } catch (e) {
        if (!cancelled) {
          setWeekData(null)
          setWeekError(typeof e === "string" ? e : "无法加载周课表")
        }
      } finally {
        if (!cancelled) setWeekLoading(false)
      }
    }
    void loadWeek()
    return () => {
      cancelled = true
    }
  }, [viewMode, semesterFilter, weekIndex, weekRefresh])

  function refreshAll() {
    void fetchCourses()
      .then(() => setActionError(null))
      .catch((e) => setActionError(userErrorMessage(e, "刷新课程列表失败")))
    setWeekRefresh((n) => n + 1)
  }

  function openCreateForm() {
    setForm({ ...emptyForm(), semester: semesterFilter })
    setEditingCourse(null)
    setShowForm(true)
  }

  function openEditForm(course: Course) {
    setForm(courseToForm(course))
    setEditingCourse(course)
    setShowForm(true)
  }

  function openImportPanel() {
    setImportError(null)
    setImportFeedback(null)
    setImportSemester(semesterFilter)
    setImportStartDate("")
    setShowImport(true)
  }

  async function handleSave() {
    if (!form.name.trim() || !form.start_time || !form.end_time) return

    setSaving(true)
    try {
      if (editingCourse) {
        const payload: UpdateCourseRequest = {
          name: form.name,
          day_of_week: form.day_of_week,
          start_time: form.start_time,
          end_time: form.end_time,
          week_pattern: form.week_pattern,
          semester_start_date: form.semester_start_date,
          location: form.location,
          teacher: form.teacher,
          color: form.color,
          semester: form.semester,
        }
        await updateCourse(editingCourse.id, payload)
      } else {
        const payload: CreateCourseRequest = {
          name: form.name,
          day_of_week: form.day_of_week,
          start_time: form.start_time,
          end_time: form.end_time,
          week_pattern: form.week_pattern || undefined,
          semester_start_date: form.semester_start_date || undefined,
          location: form.location || undefined,
          teacher: form.teacher || undefined,
          color: form.color || undefined,
          semester: form.semester || undefined,
        }
        await createCourse(payload)
      }
      setShowForm(false)
      setEditingCourse(null)
      refreshAll()
      setActionError(null)
    } catch (e) {
      setActionError(userErrorMessage(e, "保存课程失败"))
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    try {
      await deleteCourse(id)
      refreshAll()
    } catch (e) {
      setActionError(userErrorMessage(e, "删除课程失败"))
    }
  }

  async function handleReadClipboard() {
    setImportError(null)
    try {
      // 给 Android 一点时间完成剪贴板权限切换
      await new Promise((r) => setTimeout(r, 150))
      const text = await navigator.clipboard.readText()
      if (!text.trim()) {
        setImportError("剪贴板为空，请先从教务系统复制课表表格。")
        return
      }
      setImportText(text)
    } catch {
      setImportError("读取剪贴板失败，请手动粘贴课表文本后再导入。")
    }
  }

  async function handleImport() {
    if (!importText.trim()) {
      setImportError("请先粘贴或读取课表文本。")
      return
    }

    setImporting(true)
    setImportError(null)
    setImportFeedback(null)
    try {
      const result = await importCoursesFromText({
        text: importText,
        semester: importSemester.trim(),
        semester_start_date: importStartDate.trim(),
      })
      setImportFeedback(result)

      const nextSemester = importSemester.trim()
      if (nextSemester && nextSemester !== semesterFilter) {
        setSemesterFilter(nextSemester)
      } else {
        await fetchCourses(nextSemester || semesterFilter)
        setWeekRefresh((n) => n + 1)
      }
    } catch (e) {
      setImportError(typeof e === "string" ? e : "导入失败，请检查课表文本格式。")
    } finally {
      setImporting(false)
    }
  }

  // ── Inline navigation handlers ──
  const handlePrevWeek = () => {
    setWeekIndex((w) => Math.max(1, w - 1))
    setViewMode("week")
  }

  const handleNextWeek = () => {
    setWeekIndex((w) => w + 1)
    setViewMode("week")
  }

  const handleSelectWeek = (w: number) => {
    setWeekIndex(w)
    setShowWeekPicker(false)
    setViewMode("week")
  }

  const handleDayClick = (dayIndex: number, dateKey: string) => {
    setMobileDayIndex(dayIndex)
    setViewMode("week")
    const today = todayKey()
    if (dateKey === today && weekData?.semester_start_date) {
      setWeekIndex(computeCurrentWeek(weekData.semester_start_date))
    }
  }

  const handleBlockClick = (item: WeekScheduleItem) => {
    if (item.kind !== "course") return
    const course = courses.find((c) => c.id === item.id)
    if (course) {
      setDetailCourse(course)
      setShowDetail(true)
    }
  }

  const handleShowAllCourses = () => {
    setViewMode("list")
    setShowOverflowMenu(false)
  }

  async function handleResetDate() {
    if (!resetDate.trim()) return
    setResetting(true)
    try {
      await resetSemesterDates(resetDate.trim())
      setShowResetDate(false)
      setResetDate("")
      refreshAll()
    } catch {
      setActionError("重置学期起始日失败")
    } finally {
      setResetting(false)
    }
  }

  const today = todayKey()
  const currentWeek = weekData?.semester_start_date
    ? computeCurrentWeek(weekData.semester_start_date)
    : null
  const isCurrentWeek = currentWeek !== null && currentWeek === weekIndex

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* ─── Header: back + week nav + overflow ─── */}
      <div className="flex items-center gap-2 px-3 py-2 shrink-0">
        <Button
          size="icon-sm"
          variant="ghost"
          onClick={() => onNavigate("kairos")}
          aria-label="返回"
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <Button
          size="icon-sm"
          variant="ghost"
          onClick={handlePrevWeek}
          aria-label="上一周"
          className="shrink-0"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>

        {/* Week dropdown */}
        <div className="relative" ref={weekPickerRef}>
          <button
            type="button"
            onClick={() => setShowWeekPicker(!showWeekPicker)}
            className={cn(
              "inline-flex items-center gap-1 rounded-md px-2.5 py-1 text-sm font-medium transition-colors h-8",
              "text-foreground hover:bg-muted/60",
              showWeekPicker && "bg-muted",
            )}
          >
            <span className="tabular-nums">
              第 {weekIndex} 周
            </span>
            {isCurrentWeek && (
              <span className="text-[11px] text-muted-foreground font-normal">(本周)</span>
            )}
            <span className={cn("text-[10px] text-muted-foreground transition-transform ml-0.5", showWeekPicker && "rotate-180")}>▼</span>
          </button>
          {showWeekPicker && (
            <div className="absolute left-1/2 -translate-x-1/2 top-full mt-1.5 z-50 w-40 max-h-64 overflow-y-auto rounded-lg border border-border bg-card p-1 shadow-lg animate-in fade-in-0 zoom-in-95 origin-top">
              {Array.from({ length: 20 }, (_, i) => i + 1).map((w) => (
                <button
                  key={w}
                  type="button"
                  onClick={() => handleSelectWeek(w)}
                  className={cn(
                    "w-full text-left rounded-md px-3 py-1.5 text-sm transition-colors tabular-nums",
                    w === weekIndex
                      ? "bg-primary text-primary-foreground"
                      : "text-foreground hover:bg-muted/60",
                  )}
                >
                  第 {w} 周
                  {w === currentWeek && w !== weekIndex && (
                    <span className="ml-1 text-[11px] opacity-70">(本周)</span>
                  )}
                </button>
              ))}
            </div>
          )}
        </div>

        <Button
          size="icon-sm"
          variant="ghost"
          onClick={handleNextWeek}
          aria-label="下一周"
          className="shrink-0"
        >
          <ChevronRight className="h-4 w-4" />
        </Button>

        {!isCurrentWeek && currentWeek !== null && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              setWeekIndex(currentWeek)
              setViewMode("week")
            }}
            className="shrink-0 h-7 text-[11px] font-medium text-primary hover:bg-primary/10"
          >
            今天
          </Button>
        )}

        <div className="flex-1" />

        {/* Overflow menu */}
        <div className="relative" ref={overflowMenuRef}>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="更多选项"
            onClick={() => setShowOverflowMenu(!showOverflowMenu)}
            className={cn(showOverflowMenu && "bg-muted")}
          >
            <EllipsisVertical className="h-4 w-4" />
          </Button>
          {showOverflowMenu && (
            <div className="absolute right-0 top-full mt-1.5 z-50 w-44 rounded-lg border border-border bg-card p-2 shadow-lg animate-in fade-in-0 zoom-in-95 origin-top-right">
              {ENABLE_COURSE_IMPORT && (
                <button
                  type="button"
                  onClick={() => {
                    openImportPanel()
                    setShowOverflowMenu(false)
                  }}
                  className="w-full text-left rounded-md px-3 py-2 text-sm text-foreground hover:bg-muted/60 transition-colors"
                >
                  导入课表
                </button>
              )}

              <button
                type="button"
                onClick={() => {
                  setShowOverflowMenu(false)
                  setResetDate(weekData?.semester_start_date ?? "")
                  setShowResetDate(true)
                }}
                className="w-full text-left rounded-md px-3 py-2 text-sm text-foreground hover:bg-muted/60 transition-colors"
              >
                重置学期起始日
              </button>

              <button
                type="button"
                onClick={handleShowAllCourses}
                className={cn(
                  "w-full text-left rounded-md px-3 py-2 text-sm transition-colors",
                  viewMode === "list"
                    ? "bg-primary text-primary-foreground"
                    : "text-foreground hover:bg-muted/60",
                )}
              >
                全部课程
              </button>
            </div>
          )}
        </div>
      </div>

      {actionError && (
        <p className="px-3 pb-2 text-center text-sm text-destructive">
          {actionError}
        </p>
      )}

      {/* ─── Date strip (desktop: aligned with 48px + 7fr grid) ─── */}
      <div className="hidden md:block shrink-0 pb-1">
        <div
          className="grid select-none"
          style={{ gridTemplateColumns: "48px repeat(7, minmax(0, 1fr))", minWidth: "680px" }}
        >
          <div />
          {DAY_SHORT.map((day, i) => {
            const dateKey = weekData ? dayCellKey(weekData.week_start_date, i) : ""
            const dateNum = dateKey ? dateKey.slice(8) : ""
            const isToday = dateKey === today
            return (
              <button
                key={i}
                type="button"
                onClick={() => handleDayClick(i, dateKey)}
                className={cn(
                  "flex flex-col items-center justify-center h-12 text-center transition-colors",
                  isToday ? "bg-primary/10 text-primary font-semibold" : "text-foreground hover:bg-muted/40",
                )}
              >
                <span className="text-[10px] leading-tight font-medium">{day}</span>
                <span className="text-xs font-semibold leading-tight tabular-nums mt-0.5">{dateNum || "—"}</span>
              </button>
            )
          })}
        </div>
      </div>

      {/* ─── Date strip (mobile: fit all seven days inside the viewport) ─── */}
      <div className="shrink-0 overflow-x-hidden pb-1 md:hidden">
        <div
          className="grid w-full select-none"
          style={{ gridTemplateColumns: "28px repeat(7, minmax(0, 1fr))" }}
        >
          <div />
          {DAY_SHORT.map((day, i) => {
            const dateKey = weekData ? dayCellKey(weekData.week_start_date, i) : ""
            const dateNum = dateKey ? dateKey.slice(8) : ""
            const isToday = dateKey === today
            return (
              <button
                key={i}
                type="button"
                onClick={() => handleDayClick(i, dateKey)}
                className={cn(
                  "flex flex-col items-center justify-center h-12 text-center transition-colors",
                  isToday ? "bg-primary/10 text-primary font-semibold" : "text-foreground hover:bg-muted/40",
                )}
              >
                <span className="text-[10px] leading-tight font-medium">{day}</span>
                <span className="text-xs font-semibold leading-tight tabular-nums mt-0.5">{dateNum || "—"}</span>
              </button>
            )
          })}
        </div>
      </div>

      {/* ─── Content area ─── */}
      <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden pb-0 md:pb-4">
        {/* Week view */}
        {viewMode === "week" && (
          <>
            {weekLoading && (
              <div className="flex items-center justify-center py-12">
                <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
              </div>
            )}

            {!weekLoading && weekError && !weekData && (
              <div className="p-8 text-center">
                <p className="text-sm text-muted-foreground">{weekError}</p>
                <p className="mt-1 text-xs text-muted-foreground/60">
                  请先导入课表（含学期开始日期），或在课程中填写"学期开始日期"。
                </p>
              </div>
            )}

            {!weekLoading && !weekError && !weekData && (
              <p className="py-8 text-center text-sm text-muted-foreground">
                暂无课表数据
              </p>
            )}

            {!weekLoading && weekData && !weekData.courses_visible && (
              <div className="flex min-h-[280px] items-center justify-center p-6">
                <AcrylicPanel className="w-full max-w-sm p-5 text-center">
                  <p className="text-sm font-semibold text-foreground">
                    {PHASE_LABELS[weekData.phase_type]}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    当前阶段不显示课程安排
                  </p>
                  <p className="mt-2 text-[11px] text-muted-foreground/70">
                    第 {weekData.week_index} 周 · {weekData.week_start_date} 至 {weekData.week_end_date}
                  </p>
                </AcrylicPanel>
              </div>
            )}

            {!weekLoading && weekData && weekData.courses_visible && (
              <WeekGrid
                weekData={weekData}
                today={today}
                mobileDayIndex={mobileDayIndex}
                onBlockClick={handleBlockClick}
              />
            )}
          </>
        )}

        {/* List view (全部课程) */}
        {viewMode === "list" && (
          <>
            {loading && (
              <div className="flex items-center justify-center py-12">
                <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
              </div>
            )}

            {!loading && error && courses.length === 0 && (
              <p className="py-8 text-center text-sm text-muted-foreground">{error}</p>
            )}

            {!loading && courses.length === 0 && !error && (
              <div className="p-8 text-center">
                <p className="text-sm text-muted-foreground">暂无课程</p>
                <p className="mt-1 text-xs text-muted-foreground/60">
                  点击右下角 + 按钮创建第一个课程
                </p>
              </div>
            )}

            {!loading && courses.length > 0 && (
              <div className="space-y-2 p-2">
                {courses.map((course) => (
                  <AcrylicPanel
                    key={course.id}
                    className="flex items-center gap-3 bg-card p-3 transition-all hover:-translate-y-0.5 hover:bg-card/95 hover:shadow-md"
                  >
                    <div
                      className="h-3 w-3 shrink-0 rounded-full"
                      style={{ backgroundColor: course.color }}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="truncate text-sm font-semibold text-foreground">
                          {course.name}
                        </span>
                        <span className="shrink-0 text-[10px] text-muted-foreground">
                          {DAY_LABELS[course.day_of_week - 1]} {course.start_time}-
                          {course.end_time}
                        </span>
                        {course.week_pattern && (
                          <span className="shrink-0 text-[10px] text-muted-foreground/60">
                            {course.week_pattern}
                          </span>
                        )}
                      </div>
                      <div className="mt-0.5 flex items-center gap-2">
                        {course.teacher && (
                          <span className="text-xs text-muted-foreground">
                            {course.teacher}
                          </span>
                        )}
                        {course.location && (
                          <span className="text-xs text-muted-foreground/60">
                            {course.location}
                          </span>
                        )}
                        {course.semester && (
                          <span className="shrink-0 text-[10px] text-muted-foreground/60">
                            {formatSemester(course.semester)}
                          </span>
                        )}
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-9 w-9 sm:h-7 sm:w-7 text-muted-foreground hover:text-foreground"
                        aria-label={`编辑课程 ${course.name}`}
                        onClick={() => openEditForm(course)}
                      >
                        <Pencil className="h-4 w-4 sm:h-3.5 sm:w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-9 w-9 sm:h-7 sm:w-7 text-muted-foreground hover:text-destructive"
                        aria-label={`删除课程 ${course.name}`}
                        onClick={() => void handleDelete(course.id)}
                      >
                        <Trash2 className="h-4 w-4 sm:h-3.5 sm:w-3.5" />
                      </Button>
                    </div>
                  </AcrylicPanel>
                ))}
              </div>
            )}
          </>
        )}
      </div>

      {/* ─── FAB: Add course ─── */}
      <button
        type="button"
        onClick={openCreateForm}
        aria-label="添加课程"
        className={cn(
          "fixed bottom-20 md:bottom-6 right-4 md:right-6 z-40",
          "h-14 w-14 rounded-full",
          "bg-primary text-primary-foreground",
          "shadow-lg shadow-primary/25",
          "flex items-center justify-center",
          "transition-transform active:scale-95 hover:scale-105",
        )}
      >
        <Plus className="h-5 w-5" />
      </button>

      {/* ─── Course Detail Modal ─── */}
      <CourseDetailModal
        open={showDetail}
        onOpenChange={setShowDetail}
        course={detailCourse}
        onEdit={(course) => {
          setShowDetail(false)
          openEditForm(course)
        }}
        onDelete={(courseId) => {
          setShowDetail(false)
          handleDelete(courseId)
        }}
      />

      {/* ─── Course Form Modal ─── */}
      <CourseFormModal
        open={showForm}
        onOpenChange={(open) => {
          setShowForm(open)
          if (!open) setEditingCourse(null)
        }}
        isEditing={!!editingCourse}
        form={form}
        setForm={setForm}
        saving={saving}
        onSave={handleSave}
      />

      {ENABLE_COURSE_IMPORT && (
        <ImportModal
          open={showImport}
          onOpenChange={(open) => {
            setShowImport(open)
            if (!open) {
              setImportError(null)
              setImportFeedback(null)
            }
          }}
          importText={importText}
          setImportText={setImportText}
          importSemester={importSemester}
          setImportSemester={setImportSemester}
          importStartDate={importStartDate}
          setImportStartDate={setImportStartDate}
          importing={importing}
          importError={importError}
          importFeedback={importFeedback}
          onReadClipboard={handleReadClipboard}
          onImport={handleImport}
        />
      )}

      {/* ─── Reset Semester Start Date Modal ─── */}
      <Modal
        open={showResetDate}
        onOpenChange={(open) => {
          setShowResetDate(open)
          if (!open) setResetDate("")
        }}
        title="重置学期起始日"
        description="将所有课程和考试的学期开始日期统一设置为指定日期，用于修正周课表计算。"
        className="max-w-sm"
      >
        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              学期开始日期 <span className="text-destructive">*</span>
            </label>
            <input
              type="date"
              value={resetDate}
              onChange={(e) => setResetDate(e.target.value)}
              className={FIELD_CLASS}
            />
          </div>
        </div>

        <div className="mt-5 flex items-center justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setShowResetDate(false)}
          >
            取消
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={resetting || !resetDate.trim()}
            onClick={() => void handleResetDate()}
          >
            <Save className="mr-1 h-3.5 w-3.5" />
            {resetting ? "保存中..." : "确认重置"}
          </Button>
        </div>
      </Modal>
    </div>
  )
}
