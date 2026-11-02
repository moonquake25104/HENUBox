import { useState, useEffect, useCallback, useMemo } from "react"
import type { Task, TaskFilterParams, TaskPriority, CreateTaskRequest, UpdateTaskRequest } from "@/types/task"
import {
  listTasks,
  createTask,
  updateTask,
  deleteTask,
  completeDailyTask,
  uncompleteDailyTask,
} from "@/lib/api/tasks"
import { Button } from "@/components/ui/button"
import { TaskForm } from "@/components/todo/TaskForm"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { Modal } from "@/components/shared/modal"
import { Fab } from "@/components/shared/fab"
import { FilterChip } from "@/components/shared/filter-chip"
import { PageShell } from "@/components/shared/page-shell"
import { cn } from "@/lib/utils"
import { userErrorMessage } from "@/lib/errors"
import { Bell, Calendar, CheckCircle2, Circle, ListTodo, Plus, Repeat, Trash2 } from "lucide-react"

interface PriorityConfig {
  label: string
  className: string
  dotClassName: string
}

const PRIORITY_CONFIG: Record<TaskPriority, PriorityConfig> = {
  high: { label: "高", className: "bg-red-500/15 text-red-400 border-red-500/30", dotClassName: "bg-red-400" },
  medium: { label: "中", className: "bg-amber-500/15 text-amber-400 border-amber-500/30", dotClassName: "bg-amber-400" },
  low: { label: "低", className: "bg-muted text-muted-foreground border-border", dotClassName: "bg-muted-foreground" },
}

const STATUS_OPTIONS = [
  { value: "", label: "未完成" },
  { value: "todo", label: "待办" },
  { value: "in_progress", label: "进行中" },
  { value: "done", label: "已完成" },
]

const PRIORITY_OPTIONS = [
  { value: "", label: "全部优先级" },
  { value: "high", label: "高" },
  { value: "medium", label: "中" },
  { value: "low", label: "低" },
]

const PRIORITY_RANK: Record<TaskPriority, number> = {
  high: 0,
  medium: 1,
  low: 2,
}

/** +08:00 今日日期 YYYY-MM-DD（与后端 complete_daily_task 的 today_china 一致）。 */
function todayChina(): string {
  const now = new Date()
  const china = new Date(now.getTime() + now.getTimezoneOffset() * 60000 + 8 * 3600000)
  return china.toISOString().slice(0, 10)
}

function formatDueDate(dateStr: string | null): { text: string; urgent: boolean } {
  if (!dateStr) return { text: "", urgent: false }
  const due = new Date(dateStr + "T00:00:00")
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const diff = Math.floor((due.getTime() - today.getTime()) / 86400000)

  if (diff === 0) return { text: "今天", urgent: true }
  if (diff === 1) return { text: "明天", urgent: false }
  if (diff === -1) return { text: "昨天", urgent: true }
  if (diff < 0) return { text: `逾期 ${Math.abs(diff)} 天`, urgent: true }
  return { text: dateStr, urgent: false }
}

export function TaskList() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [editingTask, setEditingTask] = useState<Task | null>(null)
  const [statusFilter, setStatusFilter] = useState<string>("")
  const [priorityFilter, setPriorityFilter] = useState<string>("")

  const today = todayChina()

  // 每日任务：不受状态筛选影响、始终全量显示（含今日已完成），按优先级/标题排序。
  const dailyTasks = useMemo(() => {
    return tasks
      .filter((t) => t.is_daily)
      .sort((a, b) => {
        const pa = PRIORITY_RANK[a.priority] ?? PRIORITY_RANK.medium
        const pb = PRIORITY_RANK[b.priority] ?? PRIORITY_RANK.medium
        if (pa !== pb) return pa - pb
        return a.title.localeCompare(b.title)
      })
  }, [tasks])
  const dailyCompleted = dailyTasks.filter((t) => t.last_completed_date === today).length

  // 普通任务：默认隐藏已完成；显式选状态时按状态过滤。排序优先级高 → 低、截止日期早 → 晚。
  const normalTasks = useMemo(() => {
    const base = tasks.filter((t) => !t.is_daily)
    const visible = statusFilter ? base.filter((t) => t.status === statusFilter) : base.filter((t) => t.status !== "done")
    return [...visible].sort((a, b) => {
      const pa = PRIORITY_RANK[a.priority] ?? PRIORITY_RANK.medium
      const pb = PRIORITY_RANK[b.priority] ?? PRIORITY_RANK.medium
      if (pa !== pb) return pa - pb
      if (a.due_date && b.due_date) return a.due_date.localeCompare(b.due_date)
      if (a.due_date) return -1
      if (b.due_date) return 1
      return 0
    })
  }, [tasks, statusFilter])

  // 状态筛选在前端做（每日任务需要全量数据）；优先级筛选交给后端。
  const fetchTasks = useCallback(async () => {
    const filters: TaskFilterParams = {}
    if (priorityFilter) filters.priority_filter = priorityFilter
    const result = await listTasks(filters)
    setTasks(result)
    setError(null)
  }, [priorityFilter])

  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const filters: TaskFilterParams = {}
        if (priorityFilter) filters.priority_filter = priorityFilter
        const result = await listTasks(filters)
        if (!cancelled) {
          setTasks(result)
          setError(null)
        }
      } catch {
        if (!cancelled) {
          setError("Tauri 不可用 — 展示离线 UI")
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }
    void load()
    return () => { cancelled = true }
  }, [priorityFilter])

  async function handleCreate(data: CreateTaskRequest) {
    try {
      await createTask(data)
      setShowForm(false)
      await fetchTasks()
    } catch (e) {
      setError(userErrorMessage(e, "创建任务失败"))
    }
  }

  async function handleUpdate(data: UpdateTaskRequest) {
    if (!editingTask) return
    try {
      await updateTask(editingTask.id, data)
      setEditingTask(null)
      await fetchTasks()
    } catch (e) {
      setError(userErrorMessage(e, "更新任务失败"))
    }
  }

  async function handleDelete(id: number) {
    try {
      await deleteTask(id)
      await fetchTasks()
    } catch (e) {
      setError(userErrorMessage(e, "删除任务失败"))
    }
  }

  // 每日任务完成/取消：按 today 判断今日状态，走专用命令。
  async function handleToggleDaily(task: Task) {
    const completedToday = task.last_completed_date === today
    try {
      if (completedToday) {
        await uncompleteDailyTask(task.id)
      } else {
        await completeDailyTask(task.id)
      }
      await fetchTasks()
    } catch (e) {
      setError(userErrorMessage(e, "更新每日任务失败"))
    }
  }

  // 普通任务一次性完成。
  async function handleComplete(task: Task) {
    try {
      await updateTask(task.id, { status: "done" })
      await fetchTasks()
    } catch (e) {
      setError(userErrorMessage(e, "完成任务失败"))
    }
  }

  function openNewForm() {
    setEditingTask(null)
    setShowForm(true)
  }

  return (
    <PageShell title="待办事项" width="2xl" titleClassName="text-xl font-heading font-semibold">
      <Modal
        open={showForm || editingTask !== null}
        onOpenChange={(open) => {
          if (!open) {
            setShowForm(false)
            setEditingTask(null)
          }
        }}
        variant="center"
        title={editingTask ? "编辑任务" : "新建任务"}
        description="填写任务标题、优先级、截止日期等信息"
      >
        <TaskForm
          task={editingTask}
          onCreate={handleCreate}
          onUpdate={handleUpdate}
          onCancel={() => {
            setShowForm(false)
            setEditingTask(null)
          }}
        />
      </Modal>

      <Fab onClick={openNewForm} />

      {/* 胶囊筛选 */}
      <div className="mb-4 flex items-center gap-2">
        <FilterChip
          label="状态"
          options={STATUS_OPTIONS}
          value={statusFilter}
          onChange={setStatusFilter}
        />
        <FilterChip
          label="优先级"
          options={PRIORITY_OPTIONS}
          value={priorityFilter}
          onChange={setPriorityFilter}
        />
      </div>

      {error && tasks.length > 0 && (
        <p className="mb-3 text-center text-sm text-destructive">{error}</p>
      )}

      {loading && (
        <div className="flex items-center justify-center py-12">
          <div className="animate-spin h-6 w-6 border-2 border-primary border-t-transparent rounded-full" />
        </div>
      )}

      {!loading && error && tasks.length === 0 && (
        <p className="text-center text-sm text-muted-foreground py-8">{error}</p>
      )}

      {/* 空状态：无卡片，直接显示在背景上 */}
      {!loading && tasks.length === 0 && !error && (
        <div className="flex flex-col items-center justify-center py-16 gap-4">
          <ListTodo className="h-12 w-12 text-muted-foreground/30" strokeWidth={1.5} />
          <div className="text-center space-y-1">
            <p className="text-sm font-medium text-muted-foreground">还没有待办任务</p>
            <p className="text-xs text-muted-foreground/60">点击下方按钮开始规划</p>
          </div>
          <Button size="sm" className="min-h-11 md:min-h-8" onClick={openNewForm}>
            <Plus className="h-4 w-4 mr-1" />添加任务
          </Button>
        </div>
      )}

      {!loading && tasks.length > 0 && (
        <div className="space-y-5">
          {/* 每日任务区块 */}
          {dailyTasks.length > 0 && (
            <div>
              <div className="mb-2 flex items-center gap-2">
                <Repeat className="h-4 w-4 text-primary" />
                <h3 className="text-sm font-medium text-foreground">每日任务</h3>
                <span className="text-xs text-muted-foreground">
                  {dailyCompleted}/{dailyTasks.length} 今日已完成
                </span>
              </div>
              <div className="space-y-2">
                {dailyTasks.map((task) => {
                  const completedToday = task.last_completed_date === today
                  const priorityCfg = PRIORITY_CONFIG[task.priority] ?? PRIORITY_CONFIG.medium
                  return (
                    <AcrylicPanel
                      key={task.id}
                      className={cn(
                        "border-primary/20 bg-card p-3 cursor-pointer transition-all hover:-translate-y-0.5 hover:shadow-md",
                        completedToday && "opacity-70"
                      )}
                      onClick={() => { setEditingTask(task); setShowForm(false) }}
                    >
                      <div className="flex items-start gap-3">
                        <div className="mt-0.5 shrink-0">
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation()
                              void handleToggleDaily(task)
                            }}
                            aria-label={completedToday ? "取消今日完成" : "完成今日任务"}
                            className="flex h-11 w-11 items-center justify-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:h-6 md:w-6"
                          >
                            {completedToday ? (
                              <CheckCircle2 className="h-5 w-5 text-emerald-400" />
                            ) : (
                              <Circle className="h-5 w-5 text-muted-foreground/40 hover:text-primary/60 transition-colors" />
                            )}
                          </button>
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span
                              className={cn(
                                "text-sm font-medium truncate",
                                completedToday && "line-through text-muted-foreground"
                              )}
                            >
                              {task.title}
                            </span>
                            <span
                              className={cn(
                                "inline-flex items-center gap-1 rounded border px-1.5 py-px text-[10px] font-medium shrink-0",
                                completedToday
                                  ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/30"
                                  : "bg-primary/10 text-primary border-primary/20"
                              )}
                            >
                              {completedToday ? "今日已完成" : "今日待完成"}
                            </span>
                            <span
                              className={cn(
                                "inline-flex items-center gap-1 rounded border px-1.5 py-px text-[10px] font-medium shrink-0",
                                priorityCfg.className
                              )}
                            >
                              <span className={cn("inline-block h-1.5 w-1.5 rounded-full", priorityCfg.dotClassName)} />
                              {priorityCfg.label}
                            </span>
                          </div>
                          {task.description && (
                            <p className="text-xs text-muted-foreground mt-0.5 truncate">
                              {task.description}
                            </p>
                          )}
                          <div className="flex items-center gap-2 mt-1">
                            {task.reminder_time && (
                              <span className="inline-flex items-center gap-1 text-[10px] text-muted-foreground">
                                <Bell className="h-3 w-3" />
                                每天 {task.reminder_time} 提醒
                              </span>
                            )}
                            {task.tags && (() => {
                              try {
                                const tagArr = JSON.parse(task.tags) as string[]
                                if (!Array.isArray(tagArr) || tagArr.length === 0) return null
                                return (
                                  <span className="text-[10px] text-muted-foreground/60">
                                    {tagArr.join(" · ")}
                                  </span>
                                )
                              } catch {
                                return null
                              }
                            })()}
                          </div>
                        </div>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-11 w-11 shrink-0 text-muted-foreground hover:text-destructive md:h-7 md:w-7"
                          onClick={(e) => {
                            e.stopPropagation()
                            void handleDelete(task.id)
                          }}
                        >
                          <Trash2 className="h-4 w-4 sm:h-3.5 sm:w-3.5" />
                        </Button>
                      </div>
                    </AcrylicPanel>
                  )
                })}
              </div>
            </div>
          )}

          {/* 普通任务区块 */}
          {normalTasks.length > 0 && (
            <div>
              {dailyTasks.length > 0 && (
                <h3 className="mb-2 text-sm font-medium text-foreground">普通任务</h3>
              )}
              <div className="space-y-2">
                {normalTasks.map((task) => {
                  const dueInfo = formatDueDate(task.due_date)
                  const priorityCfg = PRIORITY_CONFIG[task.priority] ?? PRIORITY_CONFIG.medium

                  return (
                    <AcrylicPanel
                      key={task.id}
                      className={cn(
                        "p-3 cursor-pointer transition-all bg-card hover:-translate-y-0.5 hover:bg-card/95 hover:shadow-md",
                        task.status === "done" && "opacity-60"
                      )}
                      onClick={() => { setEditingTask(task); setShowForm(false) }}
                    >
                      <div className="flex items-start gap-3">
                        <div className="mt-0.5 shrink-0">
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation()
                              if (task.status === "done") return
                              void handleComplete(task)
                            }}
                            disabled={task.status === "done"}
                            aria-label={task.status === "done" ? "已完成" : "完成任务"}
                            className="flex h-11 w-11 items-center justify-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default md:h-6 md:w-6"
                          >
                            {task.status === "done" ? (
                              <CheckCircle2 className="h-5 w-5 text-emerald-400" />
                            ) : (
                              <Circle className="h-5 w-5 text-muted-foreground/40 hover:text-primary/60 transition-colors" />
                            )}
                          </button>
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span
                              className={cn(
                                "text-sm font-medium truncate",
                                task.status === "done" && "line-through text-muted-foreground"
                              )}
                            >
                              {task.title}
                            </span>
                            <span
                              className={cn(
                                "inline-flex items-center gap-1 rounded border px-1.5 py-px text-[10px] font-medium shrink-0",
                                priorityCfg.className
                              )}
                            >
                              <span
                                className={cn("inline-block h-1.5 w-1.5 rounded-full", priorityCfg.dotClassName)}
                              />
                              {priorityCfg.label}
                            </span>
                          </div>
                          {task.description && (
                            <p className="text-xs text-muted-foreground mt-0.5 truncate">
                              {task.description}
                            </p>
                          )}
                          <div className="flex items-center gap-2 mt-1">
                            {dueInfo.text && (
                              <span
                                className={cn(
                                  "inline-flex items-center gap-1 text-[10px]",
                                  dueInfo.urgent ? "text-red-400" : "text-muted-foreground"
                                )}
                              >
                                <Calendar className="h-3 w-3" />
                                {dueInfo.text}
                              </span>
                            )}
                            {task.tags && (() => {
                              try {
                                const tagArr = JSON.parse(task.tags) as string[]
                                if (!Array.isArray(tagArr) || tagArr.length === 0) return null
                                return (
                                  <span className="text-[10px] text-muted-foreground/60">
                                    {tagArr.join(" · ")}
                                  </span>
                                )
                              } catch {
                                return null
                              }
                            })()}
                          </div>
                        </div>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-11 w-11 shrink-0 text-muted-foreground hover:text-destructive md:h-7 md:w-7"
                          onClick={(e) => {
                            e.stopPropagation()
                            void handleDelete(task.id)
                          }}
                        >
                          <Trash2 className="h-4 w-4 sm:h-3.5 sm:w-3.5" />
                        </Button>
                      </div>
                    </AcrylicPanel>
                  )
                })}
              </div>
            </div>
          )}
        </div>
      )}
    </PageShell>
  )
}
