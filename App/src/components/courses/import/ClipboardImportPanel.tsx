import type { ImportTextResult } from "@/types/course-import"
import { Button } from "@/components/ui/button"
import { ClipboardPaste, Upload } from "lucide-react"
import { FIELD_CLASS } from "../utils"

export interface ClipboardImportPanelProps {
  importText: string
  setImportText: (text: string) => void
  importSemester: string
  setImportSemester: (semester: string) => void
  importStartDate: string
  setImportStartDate: (date: string) => void
  importing: boolean
  importError: string | null
  importFeedback: ImportTextResult | null
  onReadClipboard: () => void
  onImport: () => void
}

export function ClipboardImportPanel({
  importText,
  setImportText,
  importSemester,
  setImportSemester,
  importStartDate,
  setImportStartDate,
  importing,
  importError,
  importFeedback,
  onReadClipboard,
  onImport,
}: ClipboardImportPanelProps) {
  return (
    <>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_200px]">
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            课表文本 <span className="text-destructive">*</span>
          </label>
          <textarea
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            placeholder="先从教务系统网页复制课表表格，再点击「读取剪贴板」或直接粘贴到这里。"
            rows={8}
            className="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
          />
        </div>
        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">学期</label>
            <input
              type="text"
              value={importSemester}
              onChange={(e) => setImportSemester(e.target.value)}
              placeholder="如：2026S1"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              学期开始日期
            </label>
            <input
              type="date"
              value={importStartDate}
              onChange={(e) => setImportStartDate(e.target.value)}
              className={FIELD_CLASS}
            />
          </div>
          <div className="rounded-md border border-border/60 bg-background/60 p-3 text-xs text-muted-foreground space-y-1">
            <p>解析与去重在后端完成。</p>
            <p>填写开始日期可启用周视图。</p>
            <p>重复课程会被自动跳过。</p>
          </div>
        </div>
      </div>

      {importError && (
        <p className="mt-3 text-sm text-destructive">{importError}</p>
      )}
      {importFeedback && (
        <p className="mt-3 text-sm text-muted-foreground">{importFeedback.message}</p>
      )}

      <div className="mt-4 flex items-center justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => void onReadClipboard()}
        >
          <ClipboardPaste className="mr-1 h-3.5 w-3.5" />
          读取剪贴板
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={importing || !importText.trim()}
          onClick={() => void onImport()}
        >
          <Upload className="mr-1 h-3.5 w-3.5" />
          {importing ? "导入中..." : "开始导入"}
        </Button>
      </div>
    </>
  )
}
