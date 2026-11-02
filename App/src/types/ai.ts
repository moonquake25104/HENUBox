/** 与后端 commands::ai::AiConfigView 对齐。API key 永不过桥，仅返回是否已配置。 */
export interface AiConfig {
  id: number
  enabled: boolean
  /** OpenAI 兼容 API base URL，不含 /v1（后端统一追加 /v1/chat/completions）。 */
  base_url: string
  model: string
  /** 后端是否已保存 API key；明文/掩码均不回传。 */
  api_key_configured: boolean
  /** 是否开启 WebDAV 加密同步（AI 设置 + API 密钥随 WebDAV 同步）。 */
  sync_enabled: boolean
  /** UTC ISO 8601 创建时间。 */
  created_at: string
  /** UTC ISO 8601 更新时间。 */
  updated_at: string
}

/** update_ai_config 命令入参；省略字段表示保留后端当前值。 */
export interface UpdateAiConfigRequest {
  enabled?: boolean
  base_url?: string
  model?: string
  /** 有值=更新密钥；空字符串=清空；省略=保留原密钥。 */
  api_key?: string
  /** 开启/关闭 AI 设置 WebDAV 加密同步；省略=保留当前值。 */
  sync_enabled?: boolean
}

/** 摘要生成来源："ai" = deepseek 生成，"rule" = 本地规则降级。 */
export type BriefSource = "ai" | "rule"

/** 与后端 db::models::AiMorningBrief 对齐。 */
export interface AiMorningBrief {
  id: number
  /** YYYY-MM-DD（+08:00 中国日期）。 */
  date: string
  /** 固定 5 分节 markdown 子集文本（含来源 footer）。 */
  markdown: string
  source: BriefSource
  /** 仅 source === "ai" 时有值，如 "deepseek-v4-flash"。 */
  model: string
  /** 生成时间，RFC3339。 */
  generated_at: string
  created_at: string
  updated_at: string
}
