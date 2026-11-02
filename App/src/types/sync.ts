/** 与后端 commands::sync::SyncConfigView 对齐，是前端可见的 WebDAV 同步配置摘要。 */
export interface SyncConfig {
  id: number
  server_url: string
  username: string
  auto_sync: boolean
  /** 上次成功同步时间，UTC ISO 8601；null 表示尚未同步。 */
  last_sync_at: string | null
  /** 上次成功上传后服务端返回的 HTTP ETag，用于下一次 If-Match 条件上传。 */
  remote_etag: string | null
  /** 本设备 UUID，仅用于追踪快照来源，不参与合并判定。 */
  device_id: string | null
  /** 数据集 UUID，同一同步文件的所有设备共享。 */
  dataset_id: string | null
  /** 后端是否已保存 WebDAV 密码；密码原文不回传给前端。 */
  password_configured: boolean
}

/** update_sync_config 命令入参；password 省略表示保留后端已保存的密码。 */
export interface UpdateSyncConfigRequest {
  server_url: string
  username: string
  auto_sync: boolean
  /** 新密码；省略表示保留，空字符串表示清空。 */
  password?: string
}

/** 后端 sync-finished 事件 payload；只在 last_sync_at 已持久化后发送。 */
export interface SyncFinishedEvent {
  /** 上次成功同步时间，UTC ISO 8601。 */
  last_sync_at: string
}

/** 与后端 sync::exporter::SyncStats 对齐，描述一次 sync_now 的合并结果。 */
export interface SyncStats {
  /** 成功写入本地数据库的任务数（新增 + 被远端覆盖的更新）。 */
  tasks_merged: number
  /** 成功写入本地数据库的课程数。 */
  courses_merged: number
  /** 成功写入本地数据库的考试数。 */
  exams_merged: number
  /** 成功写入本地数据库的番茄钟 session 数。 */
  sessions_merged: number
  /** 成功写入本地数据库的学期阶段数。 */
  term_phases_merged: number
  /** 被拒绝的远端实体数：本地版本较新或相等时保留本地，不是传统编辑冲突。 */
  conflicts: number
}

/** sync_now 命令返回值，前端只展示同步是否发生和合并统计。 */
export interface SyncResult {
  uploaded: boolean
  downloaded: boolean
  stats: SyncStats
}
