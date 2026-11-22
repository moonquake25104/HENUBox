import { useRef, useState } from "react"
import { ChevronRight, Download, Upload } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import { restoreFocusGoal, savedFocusGoal } from "@/lib/mobile/focus-goal"
import { backupAvailable, backupErrorCode, personalBackup, type BackupPreview } from "@/lib/mobile/personal-backup"
import "./choice-picker.css"

const errors: Record<string, string> = {
  INVALID_BACKUP: "所选文件不是有效的 HENUBox 备份。",
  UNSUPPORTED_BACKUP_VERSION: "此备份版本暂不支持，请使用相同或更新版本的 HENUBox。",
  BACKUP_TOO_LARGE: "文件或记录数超过单份备份的容量上限。",
  BACKUP_PREVIEW_EXPIRED: "预览已过期，请重新选择备份文件。",
  TODO_LIMIT: "合并后待办会超过 500 条，请腾出空间后重试。",
  DOCUMENT_UNAVAILABLE: "无法打开或保存所选文件，请换一个文件或保存位置重试。",
  BACKUP_STORAGE_ERROR: "本机存储暂时不可用，请重试。同一份文件可以安全地再次导入。",
}

export function PersonalBackupSettings({ onRestored }: { onRestored: () => void }) {
  const [busy, setBusy] = useState(false)
  const [preview, setPreview] = useState<BackupPreview | null>(null)
  const [status, setStatus] = useState("")
  const [error, setError] = useState("")
  const importButton = useRef<HTMLButtonElement>(null)
  if (!backupAvailable()) return null

  const report = (exception: unknown) => {
    const code = backupErrorCode(exception)
    if (code !== "CANCELLED") setError(errors[code] ?? "备份操作未完成，请重试。")
    if (code === "BACKUP_PREVIEW_EXPIRED") setPreview(null)
  }

  async function run(operation: () => Promise<void>) {
    setStatus("")
    setError("")
    setBusy(true)
    try { await operation() } catch (e) { report(e) } finally { setBusy(false) }
  }

  function dismiss() {
    if (busy || !preview) return
    void personalBackup.discardPreview({ token: preview.token }).catch(() => {})
    setPreview(null)
    setError("")
  }

  async function restore(token: string) {
    try {
      const result = await personalBackup.restoreBackup({ token })
      let goalMessage = ""
      if (result.goal_minutes !== null) {
        try {
          goalMessage = restoreFocusGoal(result.goal_minutes) ? "已恢复每日目标。" : "保留了本机已有目标。"
        } catch { goalMessage = "每日目标未能保存，可重新导入备份。" }
      }
      setPreview(null)
      setStatus(`已新增 ${result.added_todos} 条待办、${result.added_focus_records} 条专注记录。${goalMessage}`)
    } finally {
      onRestored()
    }
  }

  return <section className="xp-group" aria-label="备份与恢复">
    <h2>备份与恢复</h2>
    <div className="xp-card">
      <p className="xp-copy">备份包含待办标题与备注、专注记录与标签、每日目标。文件可直接读取，请保存在可信的位置。</p>
      <button className="xp-row" disabled={busy} onClick={() => void run(async () => {
        const result = await personalBackup.exportBackup({ goal_minutes: savedFocusGoal() ?? -1 })
        setStatus(`已保存备份，含 ${result.todos} 条待办、${result.focus_records} 条专注记录。`)
      })}>
        <span className="xp-row-icon"><Download size={19} /></span>
        <span className="xp-row-copy"><strong>导出备份</strong><small>用系统文件选择器保存备份</small></span>
        <ChevronRight size={16} />
      </button>
      <button ref={importButton} className="xp-row" disabled={busy} onClick={() => void run(async () => {
        setPreview(await personalBackup.previewBackup())
      })}>
        <span className="xp-row-icon"><Upload size={19} /></span>
        <span className="xp-row-copy"><strong>导入恢复</strong><small>选择备份文件并预览</small></span>
        <ChevronRight size={16} />
      </button>
    </div>
    {status && <p className="xp-caption" role="status">{status}</p>}
    {error && !preview && <p className="xp-error" role="alert">{error}</p>}
    {preview && <Modal open title="恢复备份" description="同一条记录保留本机已有内容。" layer={90} className="xm-choice-modal" returnFocusRef={importButton} onOpenChange={open => { if (!open) dismiss() }}>
      <div className="space-y-2 text-sm">
        <p>备份时间 {new Date(preview.exported_at).toLocaleString("zh-CN")}</p>
        <p>{preview.todos} 条待办 · {preview.focus_records} 条专注记录</p>
        <p>每日目标 {preview.goal_minutes == null ? "未设置" : preview.goal_minutes === 0 ? "关闭" : `${preview.goal_minutes} 分钟`}</p>
        {error && <p className="xp-error" role="alert">{error}</p>}
        <button className="xm-primary mt-4 w-full" disabled={busy} onClick={() => void run(() => restore(preview.token))}>
          {busy ? "正在恢复…" : "合并到本机"}
        </button>
      </div>
    </Modal>}
  </section>
}
