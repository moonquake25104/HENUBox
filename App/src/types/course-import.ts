/** 与后端 importers::ImportTextResult 对齐（src-tauri/src/importers.rs）。 */
export interface ImportTextResult {
  /** 从剪贴板文本中成功解析出的候选记录数。 */
  parsed: number
  /** 实际写入数据库的记录数。 */
  imported: number
  /** 因导入去重规则跳过的记录数。 */
  skipped: number
  /** 后端生成的中文摘要，可直接展示给用户。 */
  message: string
}
