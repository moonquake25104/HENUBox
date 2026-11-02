import { useCallback, useEffect, useRef, useState, type ReactNode } from "react"
import type { LucideIcon } from "lucide-react"
import {
  BookOpen,
  CalendarClock,
  ListTodo,
  RefreshCw,
  Sparkles,
  Target,
  Timer,
  Wand2,
} from "lucide-react"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { userErrorMessage } from "@/lib/errors"
import { generateMorningBriefStream } from "@/lib/api/sse"
import { getAiConfig, getMorningBrief } from "@/lib/api/ai"
import { cleanLines, renderInline, renderLines } from "@/lib/markdown-inline"
import type { AiConfig, AiMorningBrief } from "@/types/ai"
import type { TodayBriefingResponse } from "@/types/briefing"

// ─── 固定 5 分节 markdown → 结构化分节 ────────────────────────────────────────

/** 解析后的一节：节标题 + 原始 markdown 块列表（段落/列表）。 */
interface BriefSection {
  title: string
  blocks: string[]
}

/** 跳过尾部 footer（含流式中未写完的部分）与 --- 分隔线，避免混入正文。
 * 兼容 `---\n*AI生成，请核实*` 合并块：所有非空行都须是分隔线或 footer 行。 */
function isFooterOrSeparator(block: string): boolean {
  const lines = block
    .split("\n")
    .map((line) => line.replace(/[*\s]/g, "").trim())
    .filter(Boolean)
  if (lines.length === 0) return true
  return lines.every(
    (line) => line === "---" || line.startsWith("AI生成") || line.startsWith("本地生成"),
  )
}

// ─── 结构化磁贴 ────────────────────────────────────────────────────────────────

/** 磁贴基础：图标 + 内容，可点击导航到对应页面。 */
function Tile({
  icon: Icon,
  iconClass,
  title,
  onClick,
  children,
}: {
  icon: LucideIcon
  iconClass: string
  title: string
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex min-h-[3.25rem] flex-col gap-0.5 rounded-lg border border-border/50 bg-card/50 px-3 py-2.5 text-left",
        "transition-colors hover:bg-card active:bg-muted/60",
      )}
    >
      <span className="flex items-center gap-1.5">
        <Icon className={cn("h-4 w-4 shrink-0", iconClass)} />
        <span className="truncate text-xs font-semibold text-foreground">{title}</span>
      </span>
      <span className="min-w-0 text-xs leading-snug text-muted-foreground">{children}</span>
    </button>
  )
}

/** 课程磁贴：今日节数 + 当前/下一节。 */
function CourseTile({
  courses,
  onNavigate,
}: {
  courses: TodayBriefingResponse["courses"]
  onNavigate: (key: string) => void
}) {
  const line = courses.today_count === 0
    ? "暂无课程安排"
    : courses.current_course
      ? `当前 ${courses.current_course.title} ${courses.current_course.start_time}`
      : courses.next_course
        ? `下一节 ${courses.next_course.title} ${courses.next_course.start_time}`
        : `今日共 ${courses.today_count} 节`
  return (
    <Tile icon={BookOpen} iconClass="text-blue-500" title="课程" onClick={() => onNavigate("courses")}>
      {line}
    </Tile>
  )
}

/** 待办磁贴：逾期/今日到期计数 + 最高优先级 spotlight。 */
function TaskTile({
  tasks,
  onNavigate,
}: {
  tasks: TodayBriefingResponse["tasks"]
  onNavigate: (key: string) => void
}) {
  const counts = [
    tasks.overdue_count > 0 && `${tasks.overdue_count} 项逾期`,
    tasks.due_today_count > 0 && `${tasks.due_today_count} 项今日到期`,
    tasks.daily_unfinished_count > 0 && `${tasks.daily_unfinished_count} 个每日任务`,
  ].filter(Boolean)
  const spotlight = tasks.spotlight[0]?.title ?? tasks.daily_spotlight[0]?.title
  const line = counts.length > 0
    ? `${counts.join("，")}${spotlight ? ` · ${spotlight}` : ""}`
    : spotlight ?? "暂无到期待办"
  return (
    <Tile icon={ListTodo} iconClass="text-amber-500" title="待办" onClick={() => onNavigate("todo")}>
      {line}
    </Tile>
  )
}

/** 考试磁贴：最近考试课程 + 剩余天数。 */
function ExamTile({
  exam,
  onNavigate,
}: {
  exam: TodayBriefingResponse["exam"]
  onNavigate: (key: string) => void
}) {
  const line = exam
    ? `${exam.course_name}${exam.days_until === 0 ? " · 今天考试" : ` · 剩余 ${exam.days_until} 天`}`
    : "近期没有考试"
  return (
    <Tile icon={CalendarClock} iconClass="text-purple-500" title="考试" onClick={() => onNavigate("exams")}>
      {line}
    </Tile>
  )
}

/** 专注磁贴：番茄钟运行状态。 */
function PomodoroTile({
  pomodoro,
  onNavigate,
}: {
  pomodoro: TodayBriefingResponse["pomodoro"]
  onNavigate: (key: string) => void
}) {
  const line = pomodoro.is_running
    ? `专注中 · 已完成 ${pomodoro.completed_sessions} 个番茄钟`
    : `可以开始今天的第一轮专注${pomodoro.completed_sessions > 0 ? `（已累计 ${pomodoro.completed_sessions} 个）` : ""}`
  return (
    <Tile icon={Timer} iconClass="text-emerald-500" title="专注" onClick={() => onNavigate("pomodoro")}>
      {line}
    </Tile>
  )
}

// ─── AI 今日重点 ──────────────────────────────────────────────────────────────

const FOCUS_STYLE: { icon: LucideIcon; iconClass: string } = {
  icon: Target,
  iconClass: "text-primary",
}

/**
 * 从 5 分节 markdown 提取「今日重点」分节（AI 唯一增值内容）。
 * 只解析到该节即返回，避免流式期间对后续节做无用全量解析（每 chunk 高频调用）。
 */
function extractFocusSection(md: string): BriefSection | null {
  for (const raw of md.split(/\n{2,}/)) {
    const block = raw.trim()
    if (!block || isFooterOrSeparator(block)) continue
    if (block.startsWith("## 今日重点")) {
      const lines = cleanLines(block.split("\n"))
      return { title: lines[0].slice(3).trim(), blocks: lines.slice(1) }
    }
    // 流式开始时若其它节先出现（异常顺序），没有今日重点则提前返回。
    if (block.startsWith("## ")) return null
  }
  return null
}

/** AI 今日重点区块：图标 + 标题 + 正文（仅 AI 启用时渲染）。 */
function FocusTile({ section }: { section: BriefSection }) {
  const Icon = FOCUS_STYLE.icon
  return (
    <div className="rounded-lg border border-primary/20 bg-primary/5 px-3 py-2">
      <div className="flex items-center gap-1.5">
        <Icon className={cn("h-4 w-4 shrink-0 text-primary")} />
        <span className="truncate text-xs font-semibold text-foreground">{section.title}</span>
      </div>
      <div className="mt-1 break-words text-xs leading-snug text-foreground/90">
        <SectionBody blocks={section.blocks} />
      </div>
    </div>
  )
}

/** 节正文：段落/列表紧凑渲染（供 FocusTile 与 markdown 分节复用）。 */
function SectionBody({ blocks }: { blocks: string[] }) {
  return (
    <div className="space-y-1">
      {blocks.map((block, i) => {
        const lines = cleanLines(block.split("\n"))
        const listItems = lines.filter((l) => l.startsWith("- "))
        if (listItems.length === lines.length) {
          return (
            <ul key={i} className="list-disc space-y-0.5 pl-4">
              {listItems.map((line, j) => (
                <li key={j}>{renderInline(line.slice(2))}</li>
              ))}
            </ul>
          )
        }
        return <p key={i}>{renderLines(lines)}</p>
      })}
    </div>
  )
}

interface AiBriefCardProps {
  /** 今日首屏结构化数据；AI 未启用时作为兜底磁贴数据源。 */
  briefing: TodayBriefingResponse
  onNavigate: (key: string) => void
}

/**
 * 今日页的 AI 每日摘要卡片。
 * AI 启用且有缓存/流式文本时渲染 5 分节 markdown；
 * AI 未启用或未配置 key 时退化为 briefing 结构化磁贴（课程/待办/考试/专注）。
 */
export function AiBriefCard({ briefing, onNavigate }: AiBriefCardProps) {
  const [config, setConfig] = useState<AiConfig | null>(null)
  const [brief, setBrief] = useState<AiMorningBrief | null>(null)
  const [generating, setGenerating] = useState(false)
  const [streamingText, setStreamingText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  // ── 初始加载：config + 今日缓存 ──
  useEffect(() => {
    let disposed = false

    async function load() {
      try {
        const cfg = await getAiConfig()
        if (disposed) return
        setConfig(cfg)
        if (!cfg.enabled || !cfg.api_key_configured) {
          return
        }
        const cached = await getMorningBrief()
        if (disposed) return
        setBrief(cached)
      } catch (err) {
        if (disposed) return
        setError(userErrorMessage(err, "无法加载 AI 摘要"))
      }
    }

    void load()
    return () => {
      disposed = true
    }
  }, [])

  // ── 手动生成 / 重新生成（流式）──
  const mountedRef = useRef(true)
  useEffect(() => {
    return () => {
      mountedRef.current = false
    }
  }, [])

  const handleGenerate = useCallback(async (force: boolean) => {
    setGenerating(true)
    setError(null)
    setStreamingText("")
    try {
      await generateMorningBriefStream({
        force,
        onDelta: (delta) => {
          // 卸载后不再追加文本，避免空转更新触发 setState
          if (!mountedRef.current) return
          setStreamingText((prev) => (prev ?? "") + delta)
        },
      })
      if (!mountedRef.current) return
      const result = await getMorningBrief()
      if (!mountedRef.current) return
      setBrief(result)
      setStreamingText(null)
    } catch (err) {
      if (!mountedRef.current) return
      setError(userErrorMessage(err, "生成摘要失败"))
    } finally {
      if (mountedRef.current) setGenerating(false)
    }
  }, [])

  const aiEnabled = Boolean(config?.enabled && config.api_key_configured)
  const markdown =
    aiEnabled && (brief?.markdown ?? (generating && streamingText ? streamingText : null))
  const focus = markdown ? extractFocusSection(markdown) : null

  return (
    <AcrylicPanel className={cn("bg-card p-4 sm:p-5", aiEnabled && "border-primary/15")}>
      {aiEnabled && (
        <div className="mb-3 flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <span className="flex h-6 w-6 items-center justify-center rounded-md bg-primary/10 text-primary">
              <Sparkles className="h-3.5 w-3.5" />
            </span>
            <h2 className="text-sm font-semibold text-foreground">今日概览</h2>
          </div>
          {brief && (
            <div className="flex items-center gap-2">
              <span
                className={cn(
                  "rounded-full px-2 py-0.5 text-[10px] font-medium",
                  brief.source === "ai"
                    ? "bg-primary/10 text-primary"
                    : "bg-muted text-muted-foreground",
                )}
              >
                {brief.source === "ai" ? "AI 生成" : "本地生成"}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="重新生成"
                onClick={() => void handleGenerate(true)}
                disabled={generating}
              >
                <RefreshCw className={cn("h-3.5 w-3.5", generating && "animate-spin")} />
              </Button>
            </div>
          )}
        </div>
      )}

      {/* AI 今日重点（唯一 AI 增值内容；流式中边生成边渲染） */}
      {focus && (
        <div className="mb-2">
          <FocusTile section={focus} />
        </div>
      )}

      {/* 结构化数据 tiles：始终可见 */}
      <div className="grid grid-cols-2 gap-2">
        <CourseTile courses={briefing.courses} onNavigate={onNavigate} />
        <TaskTile tasks={briefing.tasks} onNavigate={onNavigate} />
        <ExamTile exam={briefing.exam} onNavigate={onNavigate} />
        <PomodoroTile pomodoro={briefing.pomodoro} onNavigate={onNavigate} />
      </div>

      {aiEnabled && !brief && !(generating && streamingText) && (
        <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs text-muted-foreground">
            今天还没有生成学习建议。点击生成，由本地数据给出今日优先级引导。
          </p>
          <Button
            type="button"
            onClick={() => void handleGenerate(false)}
            disabled={generating}
            className="w-full shrink-0 sm:w-auto"
          >
            {generating ? (
              <RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />
            ) : (
              <Wand2 className="mr-1.5 h-4 w-4" />
            )}
            生成今日建议
          </Button>
        </div>
      )}

      {error && (
        <p className="mt-3 text-xs text-destructive">
          {error} ·{" "}
          <button
            type="button"
            className="text-foreground underline underline-offset-2 hover:text-primary"
            onClick={() => onNavigate("ai-settings")}
          >
            前往 AI 设置
          </button>
        </p>
      )}

      {aiEnabled && brief && (
        <p className="mt-3 flex items-center gap-1 text-[11px] text-muted-foreground/70">
          {brief.source === "ai" && brief.model ? `模型 ${brief.model} · ` : null}
          生成于{" "}
          {new Date(brief.generated_at).toLocaleTimeString("zh-CN", {
            hour: "2-digit",
            minute: "2-digit",
          })}
        </p>
      )}

      {config !== null && !aiEnabled && (
        <p className="mt-3 text-[11px] text-muted-foreground/60">
          AI 学习建议未启用 ·{" "}
          <button
            type="button"
            className="underline underline-offset-2 hover:text-primary"
            onClick={() => onNavigate("ai-settings")}
          >
            前往设置
          </button>
        </p>
      )}
    </AcrylicPanel>
  )
}
