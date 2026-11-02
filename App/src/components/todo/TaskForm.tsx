import { useState, type FormEvent } from "react"
import type { Task, CreateTaskRequest, UpdateTaskRequest, TaskPriority, TaskStatus } from "@/types/task"
import { Button } from "@/components/ui/button"
import { Save } from "lucide-react"

interface TaskFormProps {
  task?: Task | null
  onCreate: (task: CreateTaskRequest) => void | Promise<void>
  onUpdate: (task: UpdateTaskRequest) => void | Promise<void>
  onCancel: () => void
}

const PRIORITY_OPTIONS: { value: TaskPriority; label: string }[] = [
  { value: "high", label: "高" },
  { value: "medium", label: "中" },
  { value: "low", label: "低" },
]

const STATUS_OPTIONS: { value: TaskStatus; label: string }[] = [
  { value: "todo", label: "待办" },
  { value: "in_progress", label: "进行中" },
  { value: "done", label: "已完成" },
]

export function TaskForm({ task, onCreate, onUpdate, onCancel }: TaskFormProps) {
  const [title, setTitle] = useState(task?.title ?? "")
  const [description, setDescription] = useState(task?.description ?? "")
  const [status, setStatus] = useState<TaskStatus>(task?.status ?? "todo")
  const [priority, setPriority] = useState<TaskPriority>(task?.priority ?? "medium")
  const [dueDate, setDueDate] = useState(task?.due_date ?? "")
  const [tags, setTags] = useState(() => {
    if (!task?.tags) return ""
    try {
      const parsed = JSON.parse(task.tags) as string[]
      return Array.isArray(parsed) ? parsed.join(", ") : ""
    } catch {
      return task.tags
    }
  })
  const [isDaily, setIsDaily] = useState(task?.is_daily ?? false)
  const [reminderTime, setReminderTime] = useState(task?.reminder_time ?? "")
  // 后端存 "YYYY-MM-DD HH:MM"，datetime-local 需要 "YYYY-MM-DDTHH:MM"。
  const [remindAt, setRemindAt] = useState(task?.remind_at ? task.remind_at.replace(" ", "T") : "")
  const [saving, setSaving] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!title.trim()) return

    setSaving(true)
    const tagList = tags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean)

    try {
      const payload = {
        title: title.trim(),
        description: description.trim() || undefined,
        status,
        priority,
        // 每日任务没有一次性截止日期（每天重复出现）。
        due_date: isDaily ? null : (dueDate || null),
        tags: JSON.stringify(tagList),
        is_daily: isDaily,
        reminder_time: isDaily && reminderTime ? reminderTime : null,
        // 普通任务一次性提醒：提交前把 datetime-local 的 T 转回空格。
        remind_at: !isDaily && remindAt ? remindAt.replace("T", " ") : null,
      }
      if (task) {
        await onUpdate(payload as UpdateTaskRequest)
      } else {
        await onCreate(payload as CreateTaskRequest)
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-3">
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">
            标题 <span className="text-destructive">*</span>
          </label>
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            required
            placeholder="任务标题"
            className="w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
          />
        </div>

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">
            描述
          </label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="任务描述（可选）"
            rows={2}
            className="w-full rounded-md border border-input bg-background px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring resize-none"
          />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1">
              优先级
            </label>
            <select
              value={priority}
              onChange={(e) => setPriority(e.target.value as TaskPriority)}
              className="w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
            >
              {PRIORITY_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1">
              状态
            </label>
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value as TaskStatus)}
              className="w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
            >
              {STATUS_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>
        </div>

        {!isDaily && (
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1">
              截止日期
            </label>
            <input
              type="date"
              value={dueDate}
              onChange={(e) => setDueDate(e.target.value)}
              className="w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
            />
          </div>
        )}

        {!isDaily && (
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1">
              提醒时间
            </label>
            <input
              type="datetime-local"
              value={remindAt}
              onChange={(e) => setRemindAt(e.target.value)}
              className="w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
            />
            <p className="mt-1 text-[11px] text-muted-foreground/70">
              到点系统通知提醒你（可选，完成后自动清除）
            </p>
          </div>
        )}

        <div>
          <label className="flex cursor-pointer items-center gap-2 pt-1">
            <input
              type="checkbox"
              checked={isDaily}
              onChange={(e) => setIsDaily(e.target.checked)}
              className="h-4 w-4 shrink-0 accent-primary"
            />
            <span className="text-sm font-medium text-muted-foreground">
              每日任务
            </span>
          </label>
          <p className="mt-0.5 pl-6 text-[11px] text-muted-foreground/70">
            习惯类任务：每天在待办中重复出现，完成一次今日清零，次日自动回到待完成。
          </p>
        </div>

        {isDaily && (
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1">
              每日提醒时间
            </label>
            <input
              type="time"
              value={reminderTime}
              onChange={(e) => setReminderTime(e.target.value)}
              className="w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
            />
            <p className="mt-1 text-[11px] text-muted-foreground/70">
              到点会系统通知提醒你完成（可选，留空则不提醒）
            </p>
          </div>
        )}

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">
            标签（逗号分隔）
          </label>
          <input
            type="text"
            value={tags}
            onChange={(e) => setTags(e.target.value)}
            placeholder="如：学习, 工作, 个人"
            className="w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
          />
        </div>
      </div>

      <div className="flex items-center justify-end gap-2 pt-1">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          取消
        </Button>
        <Button type="submit" size="sm" disabled={saving || !title.trim()}>
          <Save className="h-3.5 w-3.5 mr-1" />
          {saving ? "保存中..." : "保存"}
        </Button>
      </div>
    </form>
  )
}
