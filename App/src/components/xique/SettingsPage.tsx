import { useEffect, useState, type FormEvent } from "react"
import {
  Bell,
  CalendarDays,
  Cloud,
  LogOut,
  Moon,
  Sun,
  Check,
  ArrowUpRight,
  ShieldCheck,
  Save,
} from "lucide-react"
import { useTheme } from "@/hooks/use-theme"
import { getNotifyConfig, updateNotifyConfig } from "@/lib/api/notify"
import { logout } from "@/lib/api/auth"
import { clearToken } from "@/lib/api/client"
import { userErrorMessage } from "@/lib/errors"
import type { CourseDefaults } from "@/App"
import logo from "@/assets/xique.svg"
import { PushSettings } from "./PushSettings"
import { NtfySettings } from "./NtfySettings"
import { detachPushDevice } from "@/lib/api/push"
import { nativeApp, apiOrigin } from "@/lib/mobile/platform"
import { NativeServerAddress } from "@/components/auth/NativeServerAddress"

export function SettingsPage({
  defaults,
  saveDefaults,
  connected,
  readAt,
  schoolStatus,
  openSchool,
}: {
  defaults: CourseDefaults
  saveDefaults: (value: CourseDefaults) => void
  connected: boolean
  readAt: Date | null
  schoolStatus: string
  openSchool: () => void
}) {
  const { theme, setTheme } = useTheme()
  const [form, setForm] = useState(defaults)
  const [reminder, setReminder] = useState({ enabled: false, minutes: 30 })
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState("")
  const [message, setMessage] = useState("")
  const [busy, setBusy] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    let live = true
    getNotifyConfig()
      .then((config) => {
        if (!live) return
        let offsets: number[] = [30]
        try {
          offsets = JSON.parse(config.course_offsets_json) as number[]
        } catch {
          /* Keep a safe UI default. */
        }
        setReminder({ enabled: config.enabled, minutes: offsets[0] || 30 })
        setLoaded(true)
        setError("")
      })
      .catch((err) => {
        if (live) setError(userErrorMessage(err, "提醒设置读取失败"))
      })
    return () => {
      live = false
    }
  }, [retry])
  function defaultsSubmit(event: FormEvent) {
    event.preventDefault()
    try {
      saveDefaults({ semester: form.semester.trim(), anchor: form.anchor })
      setMessage("默认课程设置已保存")
    } catch {
      setError("浏览器未能保存默认设置，请检查存储权限")
    }
  }
  async function saveReminder() {
    setBusy(true)
    setMessage("")
    setError("")
    try {
      await updateNotifyConfig({
        enabled: reminder.enabled,
        course_offsets_json: JSON.stringify([reminder.minutes]),
      })
      setMessage("提醒偏好已保存；课程自动提醒接通后生效")
    } catch (err) {
      setError(userErrorMessage(err, "提醒偏好保存失败"))
    } finally {
      setBusy(false)
    }
  }
  async function signOut() {
    setLoggingOut(true)
    if (nativeApp) {
      try {
        await clearToken()
      } catch {
        setError("本机登录未清理完成，请在恢复页面重试。")
      } finally {
        setLoggingOut(false)
      }
      return
    }
    try {
      await detachPushDevice()
    } catch (err) {
      setError(userErrorMessage(err, "设备提醒未能停用，请联网后重试退出"))
      setLoggingOut(false)
      return
    }
    await logout().catch(() => {})
    await clearToken()
  }
  return (
    <>
      <div className="xq-page-heading">
        <div>
          <p className="xq-eyebrow">MAKE ROOM FOR YOUR OWN RHYTHM</p>
          <h1>
            按你的习惯来<span className="xq-title-dot">。</span>
          </h1>
          <p className="xq-subtitle">把细节安排好，让每一天更轻松。</p>
        </div>
        <span className="xq-status-pill neutral">HENUBox v0.1</span>
      </div>
      {message && (
        <div className="xq-success" role="status">
          <Check size={17} />
          {message}
        </div>
      )}
      {error && (
        <div className="xq-error" role="alert">
          <p>{error}</p>
          <button className="xq-text-button" onClick={() => setRetry((value) => value + 1)}>
            重新读取提醒设置
          </button>
        </div>
      )}
      <div className="xq-settings-grid">
        <div className="xq-settings-main">
          {nativeApp && (
            <section className="xq-panel xq-settings-section">
              <NativeServerAddress onSaved={() => {}} />
              <p className="xq-school-note">
                当前同步服务：{apiOrigin}。成功读取后，整学期课表保存在手机，离线也能查看。修改与更新仍需连接服务。
              </p>
            </section>
          )}
          {!nativeApp && <NtfySettings />}
          {!nativeApp && (
            <details className="xq-native-browser-option">
              <summary>网页推送兼容性测试（可选）</summary>
              <PushSettings />
            </details>
          )}
          <section className="xq-panel xq-settings-section">
            <div className="xq-section-heading">
              <div className="xq-setting-title">
                <span className="xq-stat-icon blue">
                  <Cloud size={21} />
                </span>
                <div>
                  <h2>河南大学教务</h2>
                  <p>{schoolStatus}</p>
                </div>
              </div>
              <button className="xq-button small" onClick={openSchool}>
                管理学校连接
              </button>
            </div>
            <p className="xq-school-note">检查学校当前学期的课表，核对变化后再确认更新。</p>
          </section>
          <section className="xq-panel xq-settings-section">
            <div className="xq-section-heading">
              <div className="xq-setting-title">
                <span className="xq-stat-icon blue">
                  <CalendarDays size={21} />
                </span>
                <div>
                  <h2>新建课程的默认值</h2>
                  <p>填写课程时，少做一点重复的事。</p>
                </div>
              </div>
            </div>
            <form onSubmit={defaultsSubmit}>
              <div className="xq-form-row">
                <div className="xq-field">
                  <label htmlFor="xq-default-semester">默认学期</label>
                  <input
                    id="xq-default-semester"
                    required
                    maxLength={60}
                    value={form.semester}
                    onChange={(event) => setForm({ ...form, semester: event.target.value })}
                    placeholder="例如：2026S2"
                  />
                </div>
                <div className="xq-field">
                  <label htmlFor="xq-default-anchor">第一教学周所在日期</label>
                  <input
                    id="xq-default-anchor"
                    type="date"
                    required
                    value={form.anchor}
                    onChange={(event) => setForm({ ...form, anchor: event.target.value })}
                  />
                </div>
              </div>
              <div className="xq-setting-save">
                <p>保存在当前浏览器，用于新课程。已有课程可在课表中单独修改。</p>
                <button
                  className="xq-button primary small"
                  type="submit"
                  disabled={!form.semester.trim()}
                >
                  <Save size={14} />
                  保存默认值
                </button>
              </div>
            </form>
          </section>
          <section className="xq-panel xq-settings-section">
            <div className="xq-section-heading">
              <div className="xq-setting-title">
                <span className="xq-stat-icon violet">
                  <Bell size={21} />
                </span>
                <div>
                  <h2>上课提醒</h2>
                  <p>为出发，留一点准备的时间。</p>
                </div>
              </div>
              <span className="xq-status-pill">课程提醒待接入</span>
            </div>
            <div className="xq-setting-line">
              <div>
                <strong>保存上课提醒偏好</strong>
                <p>提醒偏好保存在服务端，在其他设备也可读取。</p>
              </div>
              <label className="xq-switch">
                <input
                  type="checkbox"
                  aria-label="启用上课提醒偏好"
                  disabled={!loaded || busy}
                  checked={reminder.enabled}
                  onChange={(event) => setReminder({ ...reminder, enabled: event.target.checked })}
                />
                <span />
              </label>
            </div>
            <div className="xq-setting-line">
              <label htmlFor="xq-reminder-minutes">提前多久提醒</label>
              <select
                id="xq-reminder-minutes"
                className="xq-small-select"
                disabled={!loaded || busy}
                value={reminder.minutes}
                onChange={(event) =>
                  setReminder({ ...reminder, minutes: Number(event.target.value) })
                }
              >
                {[...new Set([5, 10, 15, 30, 60, reminder.minutes])]
                  .sort((a, b) => a - b)
                  .map((value) => (
                    <option key={value} value={value}>
                      提前 {value} 分钟
                    </option>
                  ))}
              </select>
            </div>
            <div className="xq-setting-save">
              <p>手机测试通知已单独提供；课程自动提醒尚未接入。</p>
              <button
                className="xq-button small"
                disabled={!loaded || busy}
                onClick={() => void saveReminder()}
              >
                {busy ? "正在保存…" : "保存提醒偏好"}
              </button>
            </div>
          </section>
          <section className="xq-panel xq-settings-section">
            <div className="xq-section-heading">
              <div className="xq-setting-title">
                <span className="xq-stat-icon green">
                  <Sun size={21} />
                </span>
                <div>
                  <h2>外观</h2>
                  <p>白天清爽，夜晚柔和。</p>
                </div>
              </div>
            </div>
            <div className="xq-theme-options">
              {[
                { value: "light" as const, label: "清爽浅色", icon: Sun },
                { value: "dark" as const, label: "安静深色", icon: Moon },
              ].map(({ value, label, icon: Icon }) => (
                <button
                  key={value}
                  aria-pressed={theme === value}
                  className={theme === value ? "selected" : ""}
                  onClick={() => setTheme(value)}
                >
                  <span className={`xq-theme-preview ${value}`}>
                    <i />
                    <i />
                    <i />
                  </span>
                  <span>
                    <Icon size={16} />
                    {label}
                    {theme === value && <Check size={15} />}
                  </span>
                </button>
              ))}
            </div>
          </section>
        </div>
        <aside className="xq-settings-aside">
          <section className="xq-panel xq-settings-section">
            <div className="xq-about-brand">
              <img src={logo} alt="HENUBox标识" />
              <h2>HENUBox</h2>
              <p>你的个人教务工作台</p>
            </div>
            <div className="xq-service-line">
              <Cloud size={16} />
              <span>课程数据服务</span>
              <strong className={connected ? "xq-green-text" : ""}>
                {connected ? "已连接" : "未连接"}
              </strong>
            </div>
            <div className="xq-service-line">
              <ShieldCheck size={16} />
              <span>教务系统连接</span>
              <strong>{schoolStatus}</strong>
            </div>
            <p className="xq-read-time">
              {readAt
                ? `最近读取 ${readAt.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", timeZone: "Asia/Shanghai" })}`
                : "尚未读取课程数据"}
            </p>
            <div className="xq-info-note">
              支持手动课程与教务文件导入；学校连接和更新检查可在上方管理。
            </div>
          </section>
          <section className="xq-panel xq-account-card">
            <div>
              <span className="xq-avatar">雀</span>
              <div>
                <strong>我的个人空间</strong>
                <p>仅供本人使用</p>
                {nativeApp && <p>退出登录会清除本机课表</p>}
              </div>
            </div>
            <button
              className="xq-text-button destructive"
              disabled={loggingOut}
              onClick={() => void signOut()}
            >
              <LogOut size={16} />
              {loggingOut ? "正在退出…" : "退出登录"}
              <ArrowUpRight size={14} />
            </button>
          </section>
        </aside>
      </div>
    </>
  )
}
