import { useEffect, useState } from "react"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { AiBriefCard } from "@/pages/today/AiBriefCard"
import type { TodayBriefingResponse } from "@/types/briefing"
import { getTodayBriefing } from "@/lib/api/briefing"

const PHASE_LABELS: Record<TodayBriefingResponse["phase"]["phase_type"], string> = {
  unknown: "未识别阶段",
  teaching: "教学周",
  exam: "考试周",
  break: "假期",
}

interface TodayPageProps {
  onNavigate: (key: string) => void
}

export function TodayPage({ onNavigate }: TodayPageProps) {
  const [briefing, setBriefing] = useState<TodayBriefingResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getTodayBriefing()
      .then(setBriefing)
      .catch((err: string) => setError(err))
  }, [])

  if (error) {
    return (
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 py-6">
        <AcrylicPanel className="bg-card p-6">
          <p className="text-sm text-muted-foreground">今日概览加载失败</p>
        </AcrylicPanel>
      </div>
    )
  }

  if (!briefing) {
    return (
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 py-6">
        <AcrylicPanel className="bg-card p-6">
          <div className="h-5 w-36 animate-pulse rounded bg-muted" />
          <div className="mt-4 grid grid-cols-2 gap-2">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="h-14 animate-pulse rounded-lg bg-muted/60" />
            ))}
          </div>
        </AcrylicPanel>
      </div>
    )
  }

  const { date, weekday_label, phase } = briefing

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 py-6">
      {/* 页头：今天 + 日期 + 学期阶段 */}
      <AcrylicPanel className="bg-card p-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="font-heading text-lg font-semibold text-foreground">今天</h1>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {date} {weekday_label}
            </p>
          </div>
          <div className="rounded-md border border-border/60 bg-muted/40 px-2.5 py-1 text-right">
            <p className="text-xs font-medium text-foreground">{PHASE_LABELS[phase.phase_type]}</p>
            {phase.current_week && (
              <p className="mt-0.5 text-[10px] text-muted-foreground">第 {phase.current_week} 周</p>
            )}
          </div>
        </div>
      </AcrylicPanel>

      {/* 今日概览：结构化数据 + AI 今日重点（AI 未启用时仅结构化数据） */}
      <AiBriefCard briefing={briefing} onNavigate={onNavigate} />
    </div>
  )
}