import { useState, useEffect, useCallback, useRef } from "react"
import { ArrowLeft, Bell, BellOff } from "lucide-react"
import type { NotificationConfig, UpdateNotificationConfig } from "@/types/notification"
import { Button } from "@/components/ui/button"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { cn } from "@/lib/utils"
import { getNotifyConfig, updateNotifyConfig } from "@/lib/api/notify"

interface NotificationSettingsProps {
  onNavigate: (key: string) => void
}

/** 考试提醒偏移选项，单位分钟。 */
const OFFSET_OPTIONS = [
  { label: "提前 1 天", value: 1440 },
  { label: "提前 1 小时", value: 60 },
  { label: "提前 30 分钟", value: 30 },
  { label: "提前 10 分钟", value: 10 },
]

/** 课程提醒偏移选项，单位分钟。 */
const COURSE_OFFSET_OPTIONS = [
  { label: "提前 1 小时", value: 60 },
  { label: "提前 30 分钟", value: 30 },
  { label: "提前 10 分钟", value: 10 },
  { label: "提前 5 分钟", value: 5 },
]

function parseOffsets(json: string): number[] {
  try {
    const arr = JSON.parse(json)
    if (Array.isArray(arr) && arr.every((v) => typeof v === "number" && v > 0)) {
      return arr
    }
  } catch {
    // fall through
  }
  return [1440, 60]
}

function offsetsToJson(offsets: number[]): string {
  return JSON.stringify(offsets)
}

export function NotificationSettings({ onNavigate }: NotificationSettingsProps) {
  const [config, setConfig] = useState<NotificationConfig | null>(null)
  const [enabled, setEnabled] = useState(true)
  const [selectedOffsets, setSelectedOffsets] = useState<number[]>([1440, 60])
  const [courseOffsets, setCourseOffsets] = useState<number[]>([30])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const latestDraftRef = useRef<UpdateNotificationConfig | null>(null)
  const lastPersistedRef = useRef<UpdateNotificationConfig | null>(null)

  // 初始加载
  useEffect(() => {
    getNotifyConfig()
      .then((cfg) => {
        setConfig(cfg)
        setEnabled(cfg.enabled)
        setSelectedOffsets(parseOffsets(cfg.exam_offsets_json))
        setCourseOffsets(parseOffsets(cfg.course_offsets_json))
        setLoading(false)
        lastPersistedRef.current = {
          enabled: cfg.enabled,
          exam_offsets_json: cfg.exam_offsets_json,
          course_offsets_json: cfg.course_offsets_json,
        }
      })
      .catch((err) => {
        setError(typeof err === "string" ? err : "无法加载通知配置")
        setLoading(false)
      })
  }, [])

  // 追踪草稿变化
  useEffect(() => {
    if (!config) return
    latestDraftRef.current = {
      enabled,
      exam_offsets_json: offsetsToJson(selectedOffsets),
      course_offsets_json: offsetsToJson(courseOffsets),
    }
  }, [config, enabled, selectedOffsets, courseOffsets])

  // 离开页面前持久化未保存的更改
  useEffect(() => {
    return () => {
      const draft = latestDraftRef.current
      const persisted = lastPersistedRef.current
      if (!draft || !persisted) return

      const hasChanges =
        draft.enabled !== persisted.enabled ||
        draft.exam_offsets_json !== persisted.exam_offsets_json ||
        draft.course_offsets_json !== persisted.course_offsets_json

      if (!hasChanges) return

      void updateNotifyConfig(draft).then(() => {
        lastPersistedRef.current = draft
      })
    }
  }, [])

  const handleEnabledToggle = useCallback(async () => {
    const nextEnabled = !enabled
    setEnabled(nextEnabled)
    const req: UpdateNotificationConfig = {
      enabled: nextEnabled,
      exam_offsets_json: offsetsToJson(selectedOffsets),
      course_offsets_json: offsetsToJson(courseOffsets),
    }
    setSaving(true)
    try {
      await updateNotifyConfig(req)
      lastPersistedRef.current = req
    } catch (err) {
      setEnabled(!nextEnabled)
      setError(typeof err === "string" ? err : "保存失败")
    } finally {
      setSaving(false)
    }
  }, [enabled, selectedOffsets, courseOffsets])

  const handleOffsetToggle = useCallback(
    async (offset: number) => {
      const next = selectedOffsets.includes(offset)
        ? selectedOffsets.filter((o) => o !== offset)
        : [...selectedOffsets, offset].sort((a, b) => b - a)
      setSelectedOffsets(next)
      const req: UpdateNotificationConfig = {
        enabled,
        exam_offsets_json: offsetsToJson(next),
        course_offsets_json: offsetsToJson(courseOffsets),
      }
      setSaving(true)
      try {
        await updateNotifyConfig(req)
        lastPersistedRef.current = req
      } catch (err) {
        setSelectedOffsets(selectedOffsets)
        setError(typeof err === "string" ? err : "保存失败")
      } finally {
        setSaving(false)
      }
    },
    [enabled, selectedOffsets, courseOffsets],
  )

  const handleCourseOffsetToggle = useCallback(
    async (offset: number) => {
      const next = courseOffsets.includes(offset)
        ? courseOffsets.filter((o) => o !== offset)
        : [...courseOffsets, offset].sort((a, b) => b - a)
      setCourseOffsets(next)
      const req: UpdateNotificationConfig = {
        enabled,
        exam_offsets_json: offsetsToJson(selectedOffsets),
        course_offsets_json: offsetsToJson(next),
      }
      setSaving(true)
      try {
        await updateNotifyConfig(req)
        lastPersistedRef.current = req
      } catch (err) {
        setCourseOffsets(courseOffsets)
        setError(typeof err === "string" ? err : "保存失败")
      } finally {
        setSaving(false)
      }
    },
    [enabled, selectedOffsets, courseOffsets],
  )

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin h-6 w-6 border-2 border-primary border-t-transparent rounded-full" />
      </div>
    )
  }

  if (error && !config) {
    return (
      <div className="min-h-0 flex-1 overflow-y-auto pb-4">
        <div className="mx-auto flex w-full max-w-md flex-col gap-4">
          <div className="flex items-start gap-2">
            <Button
              size="icon-sm"
              variant="ghost"
              onClick={() => onNavigate("kairos")}
              aria-label="返回设置"
              className="mt-0.5 shrink-0"
            >
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <div className="min-w-0">
              <h2 className="text-lg font-semibold text-foreground">通知设置</h2>
            </div>
          </div>
          <p className="text-center text-sm text-destructive">{error}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto pb-4">
      <div className="mx-auto flex w-full max-w-md flex-col gap-4 animate-in fade-in-0 slide-in-from-bottom-2 duration-300">
        <div className="flex items-start gap-2">
          <Button
            size="icon-sm"
            variant="ghost"
            onClick={() => onNavigate("kairos")}
            aria-label="返回设置"
            className="mt-0.5 shrink-0"
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="min-w-0">
            <h2 className="text-lg font-semibold text-foreground">通知设置</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              管理番茄钟提醒与考试考前通知。
            </p>
          </div>
        </div>

        <AcrylicPanel className="border-primary/15 p-4 sm:p-6">
          <h3 className="mb-4 text-sm font-semibold text-foreground">全局开关</h3>

          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">
              {enabled ? "通知已开启" : "通知已关闭"}
            </span>
            <button
              type="button"
              role="switch"
              aria-checked={enabled}
              aria-label={enabled ? "关闭通知" : "开启通知"}
              onClick={handleEnabledToggle}
              disabled={saving}
              className={cn(
                "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
                enabled ? "bg-primary" : "bg-muted-foreground/25",
              )}
            >
              <span
                className={cn(
                  "pointer-events-none flex h-5 w-5 items-center justify-center rounded-full bg-card shadow-sm ring-0 transition-transform",
                  enabled ? "translate-x-5" : "translate-x-0",
                )}
              >
                {enabled ? (
                  <Bell className="h-3 w-3 text-primary" />
                ) : (
                  <BellOff className="h-3 w-3 text-muted-foreground" />
                )}
              </span>
            </button>
          </div>
        </AcrylicPanel>

        <AcrylicPanel className="border-primary/15 p-4 sm:p-6">
          <h3 className="mb-4 text-sm font-semibold text-foreground">考试考前提醒</h3>
          <p className="mb-3 text-xs text-muted-foreground">
            选择考试开始前多久发送提醒通知，可多选。
          </p>

          <div className="flex flex-col gap-2">
            {OFFSET_OPTIONS.map(({ label, value }) => {
              const isActive = selectedOffsets.includes(value)
              return (
                <label
                  key={value}
                  className={cn(
                    "flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2.5 transition-colors",
                    "min-h-11",
                    isActive
                      ? "border-primary bg-primary/5 text-foreground"
                      : "border-border bg-muted/40 text-muted-foreground hover:bg-muted",
                  )}
                >
                  <input
                    type="checkbox"
                    checked={isActive}
                    onChange={() => handleOffsetToggle(value)}
                    disabled={saving}
                    className="h-4 w-4 shrink-0 rounded border-border text-primary accent-primary"
                  />
                  <span className="text-sm">{label}</span>
                </label>
              )
            })}
          </div>
        </AcrylicPanel>

        <AcrylicPanel className="border-primary/15 p-4 sm:p-6">
          <h3 className="mb-4 text-sm font-semibold text-foreground">课前提醒</h3>
          <p className="mb-3 text-xs text-muted-foreground">
            选择每节课开始前多久发送提醒邮件，可多选。
          </p>

          <div className="flex flex-col gap-2">
            {COURSE_OFFSET_OPTIONS.map(({ label, value }) => {
              const isActive = courseOffsets.includes(value)
              return (
                <label
                  key={value}
                  className={cn(
                    "flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2.5 transition-colors",
                    "min-h-11",
                    isActive
                      ? "border-primary bg-primary/5 text-foreground"
                      : "border-border bg-muted/40 text-muted-foreground hover:bg-muted",
                  )}
                >
                  <input
                    type="checkbox"
                    checked={isActive}
                    onChange={() => handleCourseOffsetToggle(value)}
                    disabled={saving}
                    className="h-4 w-4 shrink-0 rounded border-border text-primary accent-primary"
                  />
                  <span className="text-sm">{label}</span>
                </label>
              )
            })}
          </div>
        </AcrylicPanel>

        {error && (
          <p className="text-center text-sm text-destructive">{error}</p>
        )}
      </div>
    </div>
  )
}
