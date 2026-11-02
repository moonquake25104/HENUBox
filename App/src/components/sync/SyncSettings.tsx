import { useState, useEffect, useCallback, useRef } from "react"
import { ArrowLeft, RefreshCw, Check, X } from "lucide-react"
import type { SyncConfig, SyncResult, UpdateSyncConfigRequest } from "@/types/sync"
import { Button } from "@/components/ui/button"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { cn } from "@/lib/utils"
import { userErrorMessage } from "@/lib/errors"
import { getSyncConfig, updateSyncConfig, testSyncConnection, syncNow } from "@/lib/api/sync"

interface SyncSettingsProps {
  onNavigate: (key: string) => void
}

interface SyncDraft {
  server_url: string
  username: string
  auto_sync: boolean
}

function draftFromConfig(config: SyncConfig): SyncDraft {
  return {
    server_url: config.server_url,
    username: config.username,
    auto_sync: config.auto_sync,
  }
}

function draftsEqual(a: SyncDraft, b: SyncDraft): boolean {
  return (
    a.server_url === b.server_url &&
    a.username === b.username &&
    a.auto_sync === b.auto_sync
  )
}

function inputClass(hasError: boolean): string {
  return cn(
    "w-full rounded-lg border bg-muted/40 px-3 py-2 text-sm outline-none transition-colors",
    "placeholder:text-muted-foreground/45",
    "focus:ring-2 focus:ring-primary/30",
    hasError
      ? "border-destructive/60 focus:border-destructive focus:ring-destructive/20"
      : "border-border focus:border-primary",
  )
}

export function SyncSettings({ onNavigate }: SyncSettingsProps) {
  const [config, setConfig] = useState<SyncConfig | null>(null)
  const [serverUrl, setServerUrl] = useState("")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [passwordDirty, setPasswordDirty] = useState(false)
  const [autoSync, setAutoSync] = useState(false)
  const [lastSyncAt, setLastSyncAt] = useState<string | null>(null)

  const [testStatus, setTestStatus] = useState<"idle" | "loading" | "ok" | "fail">("idle")
  const [testMessage, setTestMessage] = useState("")

  const [syncStatus, setSyncStatus] = useState<"idle" | "loading" | "done">("idle")
  const [syncResult, setSyncResult] = useState<SyncResult | null>(null)
  const [syncError, setSyncError] = useState<string | null>(null)
  const configRef = useRef<SyncConfig | null>(null)
  const latestDraftRef = useRef<SyncDraft | null>(null)
  const lastPersistedRef = useRef<SyncDraft | null>(null)
  const passwordDraftRef = useRef("")
  const passwordDirtyRef = useRef(false)

  const applyConfig = useCallback((cfg: SyncConfig, updateEditableFields: boolean) => {
    setConfig(cfg)
    configRef.current = cfg
    setLastSyncAt(cfg.last_sync_at)

    const persistedDraft = draftFromConfig(cfg)
    lastPersistedRef.current = persistedDraft

    if (updateEditableFields) {
      setServerUrl(cfg.server_url)
      setUsername(cfg.username)
      setAutoSync(cfg.auto_sync)
      setPassword("")
      setPasswordDirty(false)
      passwordDraftRef.current = ""
      passwordDirtyRef.current = false
      latestDraftRef.current = persistedDraft
    }
  }, [])

  // ── 初始加载 ──────────────────────────────────────────
  useEffect(() => {
    let disposed = false

    getSyncConfig()
      .then((cfg) => {
        if (disposed) return
        applyConfig(cfg, true)
      })
      .catch((err) => {
        if (disposed) return
        setSyncError(userErrorMessage(err, "无法加载同步配置"))
      })

    return () => {
      disposed = true
    }
  }, [applyConfig])

  // ── 顺序化保存配置：先写库，返回 Promise<SyncConfig> ──────
  const saveConfig = useCallback(
    async (overrides?: Partial<SyncDraft>): Promise<SyncConfig> => {
      const currentConfig = configRef.current ?? config
      if (!currentConfig) {
        throw new Error("Config not loaded")
      }
      const draft: SyncDraft = {
        server_url: serverUrl,
        username,
        auto_sync: autoSync,
        ...overrides,
      }
      const request: UpdateSyncConfigRequest = {
        ...draft,
      }
      if (passwordDirty) {
        request.password = password
      }

      await updateSyncConfig(request)
      const saved = await getSyncConfig()
      applyConfig(saved, true)
      return saved
    },
    [applyConfig, autoSync, config, password, passwordDirty, serverUrl, username],
  )

  useEffect(() => {
    if (!config) return

    latestDraftRef.current = {
      server_url: serverUrl,
      username,
      auto_sync: autoSync,
    }
    passwordDraftRef.current = password
    passwordDirtyRef.current = passwordDirty
  }, [config, serverUrl, username, password, passwordDirty, autoSync])

  useEffect(() => {
    return () => {
      const currentConfig = configRef.current
      const draft = latestDraftRef.current
      const persisted = lastPersistedRef.current
      if (!currentConfig || !draft || !persisted) return

      const hasUnsavedChanges = !draftsEqual(draft, persisted) || passwordDirtyRef.current

      if (!hasUnsavedChanges) return

      const request: UpdateSyncConfigRequest = {
        ...draft,
      }
      if (passwordDirtyRef.current) {
        request.password = passwordDraftRef.current
      }

      void updateSyncConfig(request)
        .then(() => {
          lastPersistedRef.current = draft
        })
        .catch(() => undefined)
    }
  }, [])

  // ── 测试连接：先保存最新配置，再测试 ────────────────────
  const handleTestConnection = useCallback(async () => {
    setTestStatus("loading")
    setTestMessage("")
    try {
      await saveConfig()
      const ok = await testSyncConnection()
      setTestStatus(ok ? "ok" : "fail")
      setTestMessage(ok ? "连接成功" : "无法连接到服务器")
    } catch (err) {
      setTestStatus("fail")
      setTestMessage(userErrorMessage(err, "连接失败"))
    }
  }, [saveConfig])

  // ── 立即同步：先保存最新配置，再同步 ────────────────────
  const handleSyncNow = useCallback(async () => {
    setSyncStatus("loading")
    setSyncResult(null)
    setSyncError(null)
    try {
      await saveConfig()
      const result = await syncNow()
      setSyncResult(result)
      setSyncStatus("done")
      // 同步成功后重新读取 last_sync_at（避免前端猜测时间）
      const cfg = await getSyncConfig()
      applyConfig(cfg, false)
    } catch (err) {
      setSyncError(userErrorMessage(err, "同步失败"))
      setSyncStatus("done")
    }
  }, [applyConfig, saveConfig])

  // ── 自动同步开关：先保存状态，再触发后端启停 ─────────────
  const handleAutoSyncToggle = useCallback(async () => {
    const nextAutoSync = !autoSync
    setAutoSync(nextAutoSync)
    try {
      await saveConfig({ auto_sync: nextAutoSync })
    } catch (err) {
      // 保存失败时回退 UI 状态
      setAutoSync(!nextAutoSync)
      setSyncError(userErrorMessage(err, "保存自动同步设置失败"))
    }
  }, [autoSync, saveConfig])

  const passwordPlaceholder =
    config?.password_configured && !passwordDirty
      ? "已保存密码，留空不变"
      : "••••••••"

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
            <h2 className="text-lg font-semibold text-foreground">同步设置</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              配置 WebDAV 备份与多端同步，保护本地学习数据。
            </p>
          </div>
        </div>

        <AcrylicPanel className="border-primary/15 p-4 sm:p-6">
          <h2 className="mb-4 text-lg font-semibold text-foreground">
            WebDAV 连接
          </h2>

          <div className="flex flex-col gap-4">
            <div className="mb-3">
              <label className="mb-0.5 block text-sm text-muted-foreground">
                服务器地址
              </label>
              <input
                type="text"
                className={inputClass(false)}
                placeholder="webdav.example.com"
                value={serverUrl}
                onChange={(e) => setServerUrl(e.target.value)}
              />
            </div>

            <div className="mb-3">
              <label className="mb-0.5 block text-sm text-muted-foreground">
                用户名
              </label>
              <input
                type="text"
                className={inputClass(false)}
                placeholder="用户名"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>

            <div className="mb-3">
              <label className="mb-0.5 block text-sm text-muted-foreground">
                密码
              </label>
              <input
                type="password"
                className={inputClass(false)}
                placeholder={passwordPlaceholder}
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value)
                  setPasswordDirty(true)
                }}
              />
            </div>

            <div className="flex items-center justify-between pt-1">
              <span className="text-sm text-muted-foreground">自动同步</span>
              <button
                type="button"
                role="switch"
                aria-checked={autoSync}
                onClick={handleAutoSyncToggle}
                className={cn(
                  "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
                  autoSync ? "bg-primary" : "bg-muted-foreground/25",
                )}
              >
                <span
                  className={cn(
                    "pointer-events-none block h-5 w-5 rounded-full bg-card shadow-sm ring-0 transition-transform",
                    autoSync ? "translate-x-5" : "translate-x-0",
                  )}
                />
              </button>
            </div>

            <div className="flex flex-col gap-3 pt-2 sm:flex-row">
              <Button
                variant="outline"
                onClick={handleTestConnection}
                disabled={testStatus === "loading" || !serverUrl}
                className="min-h-11 flex-1 border-primary/40 text-primary hover:bg-primary/5 md:min-h-0"
              >
                {testStatus === "loading" ? (
                  <RefreshCw className="mr-1 h-4 w-4 animate-spin" />
                ) : testStatus === "ok" ? (
                  <Check className="mr-1 h-4 w-4 text-emerald-400" />
                ) : testStatus === "fail" ? (
                  <X className="mr-1 h-4 w-4 text-destructive" />
                ) : null}
                测试连接
              </Button>

              <Button
                onClick={handleSyncNow}
                disabled={syncStatus === "loading" || !serverUrl}
                className="min-h-11 flex-1 md:min-h-0"
              >
                {syncStatus === "loading" ? (
                  <RefreshCw className="mr-1 h-4 w-4 animate-spin" />
                ) : null}
                立即同步
              </Button>
            </div>
          </div>
        </AcrylicPanel>

        {testMessage && (
          <p
            className={cn(
              "text-center text-sm",
              testStatus === "ok" ? "text-emerald-400" : "text-destructive",
            )}
          >
            {testMessage}
          </p>
        )}

        {lastSyncAt && (
          <p className="text-center text-xs text-muted-foreground">
            上次同步：{new Date(lastSyncAt).toLocaleString("zh-CN")}
          </p>
        )}

        {syncResult && (
          <AcrylicPanel className="p-4">
            <h3 className="mb-2 text-sm font-medium text-foreground">同步结果</h3>
            <div className="space-y-1 text-xs text-muted-foreground">
              <p>任务: {syncResult.stats.tasks_merged} 条已合并</p>
              <p>课程: {syncResult.stats.courses_merged} 条已合并</p>
              <p>考试: {syncResult.stats.exams_merged} 条已合并</p>
              <p>番茄记录: {syncResult.stats.sessions_merged} 条已合并</p>
              {syncResult.stats.conflicts > 0 && (
                <p className="text-amber-400">
                  冲突: {syncResult.stats.conflicts} 条 (本地版本已保留)
                </p>
              )}
              <p className="pt-1 text-muted-foreground/60">
                {syncResult.uploaded ? "已上传" : "未上传"} /{" "}
                {syncResult.downloaded ? "已下载" : "无远程数据"}
              </p>
            </div>
          </AcrylicPanel>
        )}

        {syncError && (
          <p className="text-center text-sm text-destructive">{syncError}</p>
        )}
      </div>
    </div>
  )
}
