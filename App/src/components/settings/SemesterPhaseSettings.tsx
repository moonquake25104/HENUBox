import { useEffect, useMemo, useState } from "react"
import { ArrowLeft, Bell, BookOpen, Pencil, Plus, Save, Trash2, X } from "lucide-react"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { Button } from "@/components/ui/button"
import type {
  CreatePomodoroProfileRequest,
  CreateTermPhaseRequest,
  CurrentPhaseStatus,
  PomodoroProfile,
  SemesterContext,
  TermPhase,
  TermPhaseType,
} from "@/types/notification"
import { cn } from "@/lib/utils"
import { userErrorMessage } from "@/lib/errors"
import {
  createProfile,
  deleteProfile,
  listProfiles,
  updateProfile,
} from "@/lib/api/pomodoro"
import {
  createTermPhase,
  deleteTermPhase,
  getCurrentPhaseStatus,
  listSemesterContexts,
  listTermPhases,
  updateTermPhase,
} from "@/lib/api/semester"

interface SemesterPhaseSettingsProps {
  onNavigate: (key: string) => void
}

const PHASE_OPTIONS: Array<{ value: TermPhaseType; label: string }> = [
  { value: "teaching", label: "教学周" },
  { value: "exam", label: "考试周" },
  { value: "break", label: "假期" },
]

const PHASE_STYLES: Record<TermPhaseType, string> = {
  teaching: "bg-blue-500",
  exam: "bg-red-500",
  break: "bg-zinc-500",
}

const FIELD_CLASS =
  "h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground outline-none transition-colors focus:border-primary"

function defaultPhase(termLabel: string, order: number): CreateTermPhaseRequest {
  return {
    term_label: termLabel,
    phase_type: "teaching",
    start_week: 1,
    end_week: 16,
    affects_courses: true,
    affects_exam_notifications: true,
    pomodoro_profile: "default",
    notification_rules_json: "{}",
    sort_order: order,
  }
}

function defaultProfile(): CreatePomodoroProfileRequest {
  return {
    name: "",
    work_seconds: 1500,
    short_break_seconds: 300,
    long_break_seconds: 900,
    sessions_before_long_break: 4,
  }
}

function phaseToDraft(phase: TermPhase): CreateTermPhaseRequest {
  return {
    term_label: phase.term_label,
    phase_type: phase.phase_type,
    start_week: phase.start_week,
    end_week: phase.end_week,
    affects_courses: phase.affects_courses,
    affects_exam_notifications: phase.affects_exam_notifications,
    pomodoro_profile: phase.pomodoro_profile,
    notification_rules_json: phase.notification_rules_json,
    sort_order: phase.sort_order,
  }
}

function profileToDraft(profile: PomodoroProfile): CreatePomodoroProfileRequest {
  return {
    name: profile.name,
    work_seconds: profile.work_seconds,
    short_break_seconds: profile.short_break_seconds,
    long_break_seconds: profile.long_break_seconds,
    sessions_before_long_break: profile.sessions_before_long_break,
  }
}

function phaseLabel(value: string): string {
  return PHASE_OPTIONS.find((option) => option.value === value)?.label ?? "未知阶段"
}

function minutes(seconds: number): number {
  return Math.round(seconds / 60)
}

function seconds(minutesValue: number): number {
  return minutesValue * 60
}

async function fetchTermPhases(termLabel: string): Promise<TermPhase[]> {
  if (!termLabel) return []
  return listTermPhases(termLabel)
}

export function SemesterPhaseSettings({ onNavigate }: SemesterPhaseSettingsProps) {
  const [contexts, setContexts] = useState<SemesterContext[]>([])
  const [selectedTerm, setSelectedTerm] = useState("")
  const [phases, setPhases] = useState<TermPhase[]>([])
  const [profiles, setProfiles] = useState<PomodoroProfile[]>([])
  const [currentPhase, setCurrentPhase] = useState<CurrentPhaseStatus | null>(null)
  const [phaseDraft, setPhaseDraft] = useState<CreateTermPhaseRequest>(() => defaultPhase("", 0))
  const [profileDraft, setProfileDraft] = useState<CreatePomodoroProfileRequest>(() => defaultProfile())
  const [editingPhaseId, setEditingPhaseId] = useState<number | null>(null)
  const [editingProfileId, setEditingProfileId] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const selectedContext = useMemo(
    () => contexts.find((context) => context.term_label === selectedTerm) ?? null,
    [contexts, selectedTerm],
  )

  useEffect(() => {
    let active = true

    async function loadInitialData() {
      try {
        const [contextList, profileList, status] = await Promise.all([
          listSemesterContexts(),
          listProfiles(),
          getCurrentPhaseStatus("manual"),
        ])
        if (!active) return

        const nextTerm = status.term_label || contextList[0]?.term_label || ""
        setContexts(contextList)
        setProfiles(profileList)
        setCurrentPhase(status)
        setSelectedTerm(nextTerm)
        setError(null)
      } catch (err) {
        if (active) setError(userErrorMessage(err, "加载学期阶段设置失败"))
      } finally {
        if (active) setLoading(false)
      }
    }

    void loadInitialData()
    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    let active = true

    async function loadSelectedTermPhases() {
      try {
        const result = await fetchTermPhases(selectedTerm)
        if (!active) return

        setPhases(result)
        setPhaseDraft(defaultPhase(selectedTerm, result.length))
        setEditingPhaseId(null)
        setError(null)
      } catch (err) {
        if (active) setError(userErrorMessage(err, "加载阶段列表失败"))
      }
    }

    void loadSelectedTermPhases()
    return () => {
      active = false
    }
  }, [selectedTerm])

  async function handleSavePhase() {
    if (!phaseDraft.term_label || phaseDraft.start_week > phaseDraft.end_week) return
    setSaving(true)
    try {
      if (editingPhaseId === null) {
        await createTermPhase(phaseDraft)
      } else {
        await updateTermPhase(editingPhaseId, phaseDraft)
      }
      const result = await fetchTermPhases(phaseDraft.term_label)
      setPhases(result)
      setEditingPhaseId(null)
      setPhaseDraft(defaultPhase(phaseDraft.term_label, result.length))
      setError(null)
    } catch (err) {
      setError(userErrorMessage(err, editingPhaseId === null ? "创建阶段失败" : "更新阶段失败"))
    } finally {
      setSaving(false)
    }
  }

  function handleEditPhase(phase: TermPhase) {
    setEditingPhaseId(phase.id)
    setPhaseDraft(phaseToDraft(phase))
  }

  function handleCancelPhaseEdit() {
    setEditingPhaseId(null)
    setPhaseDraft(defaultPhase(selectedTerm, phases.length))
  }

  async function handleDeletePhase(id: number) {
    setSaving(true)
    try {
      await deleteTermPhase(id)
      const result = await fetchTermPhases(selectedTerm)
      setPhases(result)
      if (editingPhaseId === id) setEditingPhaseId(null)
      setPhaseDraft(defaultPhase(selectedTerm, result.length))
      setError(null)
    } catch (err) {
      setError(userErrorMessage(err, "删除阶段失败"))
    } finally {
      setSaving(false)
    }
  }

  async function handleSaveProfile() {
    if (!profileDraft.name.trim()) return
    setSaving(true)
    try {
      if (editingProfileId === null) {
        await createProfile(profileDraft)
      } else {
        await updateProfile(editingProfileId, profileDraft)
      }
      const profileList = await listProfiles()
      setProfiles(profileList)
      setEditingProfileId(null)
      setProfileDraft(defaultProfile())
      setError(null)
    } catch (err) {
      setError(userErrorMessage(err, editingProfileId === null ? "创建番茄钟配置失败" : "更新番茄钟配置失败"))
    } finally {
      setSaving(false)
    }
  }

  function handleEditProfile(profile: PomodoroProfile) {
    setEditingProfileId(profile.id)
    setProfileDraft(profileToDraft(profile))
  }

  function handleCancelProfileEdit() {
    setEditingProfileId(null)
    setProfileDraft(defaultProfile())
  }

  async function handleDeleteProfile(id: number) {
    setSaving(true)
    try {
      await deleteProfile(id)
      const profileList = await listProfiles()
      setProfiles(profileList)
      if (editingProfileId === id) setEditingProfileId(null)
      setProfileDraft(defaultProfile())
      setError(null)
    } catch (err) {
      setError(userErrorMessage(err, "删除番茄钟配置失败"))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto pb-4">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
        <div className="flex items-center gap-2">
          <Button size="icon-sm" variant="ghost" onClick={() => onNavigate("kairos")} aria-label="返回">
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="min-w-0">
            <h1 className="text-lg font-semibold text-foreground">学期阶段</h1>
            <p className="text-xs text-muted-foreground">教学周、考试周与假期行为</p>
          </div>
        </div>

        {error && (
          <AcrylicPanel className="border-destructive/30 bg-card p-3">
            <p className="text-sm text-destructive">{error}</p>
          </AcrylicPanel>
        )}

        {loading ? (
          <AcrylicPanel className="bg-card p-5">
            <div className="h-5 w-36 animate-pulse rounded bg-muted" />
            <div className="mt-4 h-24 animate-pulse rounded-lg bg-muted/60" />
          </AcrylicPanel>
        ) : (
          <>
            <AcrylicPanel className="bg-card p-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                <div className="min-w-0 flex-1">
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    学期
                  </label>
                  <select
                    value={selectedTerm}
                    onChange={(event) => setSelectedTerm(event.target.value)}
                    className={FIELD_CLASS}
                  >
                    {contexts.length === 0 && <option value="">暂无学期上下文</option>}
                    {contexts.map((context) => (
                      <option key={context.id} value={context.term_label}>
                        {context.term_label} · {context.start_date}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="rounded-md border border-border/60 bg-muted/30 px-3 py-2 text-sm">
                  <span className="font-medium text-foreground">
                    {currentPhase ? phaseLabel(currentPhase.phase_type) : "未识别阶段"}
                  </span>
                  {currentPhase?.current_week && (
                    <span className="ml-2 text-xs text-muted-foreground">
                      第 {currentPhase.current_week} 周
                    </span>
                  )}
                </div>
              </div>
              {selectedContext && (
                <p className="mt-2 text-xs text-muted-foreground">
                  起始日 {selectedContext.start_date}
                  {selectedContext.total_weeks ? ` · 共 ${selectedContext.total_weeks} 周` : ""}
                </p>
              )}
            </AcrylicPanel>

            <AcrylicPanel className="bg-card p-4">
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-sm font-semibold text-foreground">阶段列表</h2>
                <span className="text-xs text-muted-foreground">{phases.length} 个阶段</span>
              </div>

              {phases.length === 0 ? (
                <p className="rounded-lg border border-dashed border-border p-4 text-center text-sm text-muted-foreground">
                  暂无阶段
                </p>
              ) : (
                <div className="space-y-2">
                  {phases.map((phase) => (
                    <div
                      key={phase.id}
                      className="flex items-center gap-3 rounded-lg border border-border/60 bg-background/60 p-3"
                    >
                      <span className={cn("h-8 w-1.5 shrink-0 rounded-full", PHASE_STYLES[phase.phase_type])} />
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium text-foreground">
                          {phaseLabel(phase.phase_type)} · 第 {phase.start_week}-{phase.end_week} 周
                        </p>
                        <div className="mt-1 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
                          <span className="inline-flex items-center gap-1">
                            <BookOpen className="h-3 w-3" />
                            {phase.affects_courses ? "显示课程" : "隐藏课程"}
                          </span>
                          <span className="inline-flex items-center gap-1">
                            <Bell className="h-3 w-3" />
                            {phase.affects_exam_notifications ? "考试通知" : "通知静默"}
                          </span>
                          <span>{phase.pomodoro_profile}</span>
                        </div>
                      </div>
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        disabled={saving}
                        onClick={() => handleEditPhase(phase)}
                        aria-label="编辑阶段"
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        disabled={saving}
                        onClick={() => void handleDeletePhase(phase.id)}
                        aria-label="删除阶段"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  ))}
                </div>
              )}

              <div className="mt-4 grid gap-3 border-t border-border/50 pt-4 md:grid-cols-6">
                <select
                  value={phaseDraft.phase_type}
                  onChange={(event) => {
                    const phase_type = event.target.value as TermPhaseType
                    setPhaseDraft((draft) => ({
                      ...draft,
                      phase_type,
                      affects_courses: phase_type !== "break",
                      affects_exam_notifications: phase_type !== "break",
                      pomodoro_profile: phase_type === "exam" ? "intense" : phase_type === "break" ? "relaxed" : "default",
                    }))
                  }}
                  className={cn(FIELD_CLASS, "md:col-span-2")}
                >
                  {PHASE_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
                <input
                  type="number"
                  min={1}
                  value={phaseDraft.start_week}
                  onChange={(event) => setPhaseDraft((draft) => ({ ...draft, start_week: Number(event.target.value) }))}
                  className={FIELD_CLASS}
                  aria-label="起始周"
                />
                <input
                  type="number"
                  min={phaseDraft.start_week}
                  value={phaseDraft.end_week}
                  onChange={(event) => setPhaseDraft((draft) => ({ ...draft, end_week: Number(event.target.value) }))}
                  className={FIELD_CLASS}
                  aria-label="结束周"
                />
                <select
                  value={phaseDraft.pomodoro_profile}
                  onChange={(event) => setPhaseDraft((draft) => ({ ...draft, pomodoro_profile: event.target.value }))}
                  className={cn(FIELD_CLASS, "md:col-span-2")}
                >
                  {profiles.map((profile) => (
                    <option key={profile.id} value={profile.name}>
                      {profile.name}
                    </option>
                  ))}
                </select>
                <Button
                  type="button"
                  className={cn(editingPhaseId === null ? "md:col-span-6" : "md:col-span-3")}
                  disabled={saving || !selectedTerm || phaseDraft.start_week > phaseDraft.end_week}
                  onClick={() => void handleSavePhase()}
                >
                  {editingPhaseId === null ? (
                    <Plus className="mr-1.5 h-4 w-4" />
                  ) : (
                    <Save className="mr-1.5 h-4 w-4" />
                  )}
                  {editingPhaseId === null ? "添加阶段" : "保存阶段"}
                </Button>
                {editingPhaseId !== null && (
                  <Button
                    type="button"
                    variant="outline"
                    className="md:col-span-3"
                    disabled={saving}
                    onClick={handleCancelPhaseEdit}
                  >
                    <X className="mr-1.5 h-4 w-4" />
                    取消编辑
                  </Button>
                )}
              </div>
            </AcrylicPanel>

            <AcrylicPanel className="bg-card p-4">
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-sm font-semibold text-foreground">番茄钟配置</h2>
                <span className="text-xs text-muted-foreground">{profiles.length} 个配置</span>
              </div>
              <div className="grid gap-2">
                {profiles.map((profile) => (
                  <div
                    key={profile.id}
                    className="flex items-center gap-3 rounded-lg border border-border/60 bg-background/60 p-3"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground">
                        {profile.name}
                        {profile.is_builtin && (
                          <span className="ml-2 text-[11px] text-muted-foreground">内置</span>
                        )}
                      </p>
                      <p className="mt-0.5 text-xs text-muted-foreground">
                        {minutes(profile.work_seconds)} / {minutes(profile.short_break_seconds)} / {minutes(profile.long_break_seconds)} 分钟 · {profile.sessions_before_long_break} 轮
                      </p>
                    </div>
                    {!profile.is_builtin && (
                      <>
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          disabled={saving}
                          onClick={() => handleEditProfile(profile)}
                          aria-label="编辑配置"
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          disabled={saving}
                          onClick={() => void handleDeleteProfile(profile.id)}
                          aria-label="删除配置"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </>
                    )}
                  </div>
                ))}
              </div>

              <div className="mt-4 grid gap-3 border-t border-border/50 pt-4 md:grid-cols-5">
                <input
                  value={profileDraft.name}
                  onChange={(event) => setProfileDraft((draft) => ({ ...draft, name: event.target.value }))}
                  placeholder="名称"
                  className={cn(FIELD_CLASS, "md:col-span-2")}
                />
                <input
                  type="number"
                  min={1}
                  value={minutes(profileDraft.work_seconds)}
                  onChange={(event) => setProfileDraft((draft) => ({ ...draft, work_seconds: seconds(Number(event.target.value)) }))}
                  className={FIELD_CLASS}
                  aria-label="专注分钟"
                />
                <input
                  type="number"
                  min={1}
                  value={minutes(profileDraft.short_break_seconds)}
                  onChange={(event) => setProfileDraft((draft) => ({ ...draft, short_break_seconds: seconds(Number(event.target.value)) }))}
                  className={FIELD_CLASS}
                  aria-label="短休息分钟"
                />
                <input
                  type="number"
                  min={1}
                  value={minutes(profileDraft.long_break_seconds)}
                  onChange={(event) => setProfileDraft((draft) => ({ ...draft, long_break_seconds: seconds(Number(event.target.value)) }))}
                  className={FIELD_CLASS}
                  aria-label="长休息分钟"
                />
                <Button
                  type="button"
                  className={cn(editingProfileId === null ? "md:col-span-5" : "md:col-span-3")}
                  disabled={saving || !profileDraft.name.trim()}
                  onClick={() => void handleSaveProfile()}
                >
                  <Save className="mr-1.5 h-4 w-4" />
                  {editingProfileId === null ? "保存配置" : "更新配置"}
                </Button>
                {editingProfileId !== null && (
                  <Button
                    type="button"
                    variant="outline"
                    className="md:col-span-2"
                    disabled={saving}
                    onClick={handleCancelProfileEdit}
                  >
                    <X className="mr-1.5 h-4 w-4" />
                    取消编辑
                  </Button>
                )}
              </div>
            </AcrylicPanel>
          </>
        )}
      </div>
    </div>
  )
}
