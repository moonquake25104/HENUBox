import { Capacitor, registerPlugin } from "@capacitor/core"

export type BackupSummary = {
  exported_at: number
  todos: number
  focus_records: number
  goal_minutes: number | null
}

export type BackupPreview = BackupSummary & { token: string }
type RestoreResult = BackupSummary & { added_todos: number; added_focus_records: number }

interface NativeBackup {
  exportBackup(input: { goal_minutes: number }): Promise<BackupSummary>
  previewBackup(): Promise<BackupPreview>
  restoreBackup(input: { token: string }): Promise<RestoreResult>
  discardPreview(input: { token: string }): Promise<void>
}

export const personalBackup = registerPlugin<NativeBackup>("XiqueBackup")
export const backupAvailable = () => Capacitor.isNativePlatform() && Capacitor.isPluginAvailable("XiqueBackup")

export function backupErrorCode(error: unknown): string {
  return error != null && typeof error === "object" && "code" in error ? String(error.code) : ""
}
