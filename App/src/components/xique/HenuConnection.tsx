import { useState } from "react"
import {
  ArrowRight,
  CheckCircle2,
  Cloud,
  ExternalLink,
  LoaderCircle,
  RefreshCw,
  Unplug,
} from "lucide-react"
import { Modal } from "@/components/shared/modal"
import { jobActive, loginActive } from "@/lib/api/henu-connection"
import { henuStatusLabel, type HenuController } from "./use-henu-connection"
import { semesterLabel } from "./dates"
import { nativeApp } from "@/lib/mobile/platform"

function time(value?: string | null) {
  return value ? new Date(value).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" }) : "尚未检查"
}
export function HenuConnection({
  controller: h,
  semester,
  close,
  preview,
}: {
  controller: HenuController
  semester: string
  close: () => void
  preview: (id: string, semester: string) => void
}) {
  const [disconnecting, setDisconnecting] = useState(false)
  const value = h.connection
  const connected = value?.connection_state === "CONNECTED"
  const canCheck =
    value?.capabilities.direct_fetch &&
    (connected || value?.connection_state === "TEMPORARY_FAILURE")
  const working = h.requesting || h.active || h.loading
  const loginRunning = loginActive(h.login)
  const jobRunning = jobActive(h.job)
  const mobile = nativeApp || /Android|iPhone|iPad/i.test(navigator.userAgent)
  return (
    <Modal
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
      title="连接河南大学"
      description="从学校获取课表，核对变化后更新。"
      className="xq-henu-modal xq-school-modal"
    >
      <div className="xq-school-banner">
        <span className="xq-feature-icon">
          <Cloud size={24} />
        </span>
        <div>
          <h3>让课表，跟上学校的安排。</h3>
          <p>先连接，再检查；每一次更新都由你确认。</p>
        </div>
      </div>
      <div className="xq-school-status">
        <span className={`xq-status-pill ${connected ? "connected" : "neutral"}`}>
          {h.loading ? "正在读取连接…" : henuStatusLabel(value)}
        </span>
        <button className="xq-text-button" disabled={h.requesting} onClick={h.refresh}>
          <RefreshCw size={14} />
          刷新状态
        </button>
      </div>
      {h.error && (
        <p className="xq-henu-message error" role="alert">
          {h.error}
        </p>
      )}
      {value?.last_error && !h.error && (
        <p className="xq-henu-message error" role="alert">
          {value.last_error.message}
        </p>
      )}
      <dl className="xq-school-facts">
        <div>
          <dt>本次检查的目标</dt>
          <dd>{semesterLabel(semester)}</dd>
        </div>
        <div>
          <dt>学校当前学期</dt>
          <dd>{value?.current_term?.label || "连接后确认"}</dd>
        </div>
        <div>
          <dt>最近检查</dt>
          <dd>{time(value?.last_checked_at)}</dd>
        </div>
      </dl>
      <p className="xq-school-note">
        目前检查学校当前学期。连接状态来自最近验证，学校要求重新登录时会在这里提示。
      </p>
      {loginRunning && (
        <section className="xq-school-task" aria-live="polite">
          <LoaderCircle size={21} className="animate-spin" />
          <div>
            <h4>
              {h.login?.state === "VERIFYING" ? "正在验证学校会话…" : "请在新打开的 Edge 窗口登录"}
            </h4>
            <p>
              认准标题中的「HENUBox 授权 {h.login?.login_id.slice(0, 6)}
              」，直接在学校页面填写账号、密码与验证码。
            </p>
            <small>有效至 {time(h.login?.expires_at)}（北京时间）。验证成功后窗口自动关闭。</small>
            <button
              className="xq-text-button"
              disabled={h.requesting}
              onClick={() => void h.cancel()}
            >
              取消本次登录
            </button>
          </div>
        </section>
      )}
      {h.login && !loginRunning && (
        <p className="xq-school-note" role="status">
          {h.login.state === "SUCCEEDED"
            ? "授权已验证，可以检查课表更新。"
            : h.login.error?.message ||
              (h.login.state === "EXPIRED"
                ? "登录窗口已超时，请重新连接。"
                : "本次登录已结束，可重新连接。")}
        </p>
      )}
      {jobRunning && (
        <section className="xq-school-task" aria-live="polite">
          <LoaderCircle className="animate-spin" size={21} />
          <div>
            <h4>正在检查学校课表…</h4>
            <p>正在获取并比较安排，已有课表仍可使用。</p>
          </div>
        </section>
      )}
      {h.job?.state === "FAILED" && (
        <p className="xq-henu-message error" role="alert">
          {h.job.error?.message || "这次检查未完成，请重试。"}
        </p>
      )}
      {h.job?.state === "SUCCEEDED" && h.job.preview_id && (
        <section className="xq-school-result" aria-live="polite">
          <CheckCircle2 size={23} />
          <div>
            <h4>检查完成，等待你核对</h4>
            <p>{semesterLabel(h.job.semester)} · 还没有修改已保存的课表。</p>
            <button
              className="xq-button primary small"
              onClick={() => preview(h.job!.preview_id!, h.job!.semester)}
            >
              查看课表变化
              <ArrowRight size={15} />
            </button>
          </div>
        </section>
      )}
      <div className="xq-school-actions">
        <button
          className="xq-button primary"
          disabled={!canCheck || working}
          onClick={() => void h.check(semester)}
        >
          <RefreshCw size={16} />
          {jobRunning ? "正在检查…" : "检查更新"}
        </button>
        <button
          className="xq-button"
          disabled={!value?.capabilities.interactive_login || working || mobile}
          onClick={() => void h.connect()}
        >
          <ExternalLink size={16} />
          {connected ? "重新授权" : "连接河大"}
        </button>
      </div>
      <p className="xq-school-note">
        {mobile
          ? "首次连接请在运行HENUBox的 Windows 电脑上操作。"
          : "点击连接后，在这台电脑的新窗口登录。助手保存学校会话，用于后续检查课表。"}
      </p>
      {value && !value.capabilities.direct_fetch && (
        <p className="xq-henu-message">当前服务尚未启用学校直连，仍可使用教务导出文件导入课表。</p>
      )}
      {value &&
        value.connection_state !== "DISCONNECTED" &&
        value.connection_state !== "UNAVAILABLE" && (
          <div className="xq-school-disconnect">
            {disconnecting ? (
              <>
                <p>断开后清除学校会话；已确认的课表与个人修改会保留。</p>
                <button
                  className="xq-button small"
                  disabled={h.requesting}
                  onClick={() => void h.disconnect().then(() => setDisconnecting(false))}
                >
                  确认断开
                </button>
                <button
                  className="xq-text-button"
                  disabled={h.requesting}
                  onClick={() => setDisconnecting(false)}
                >
                  保留连接
                </button>
              </>
            ) : (
              <button
                className="xq-text-button"
                disabled={h.requesting}
                onClick={() => setDisconnecting(true)}
              >
                <Unplug size={14} />
                断开学校连接
              </button>
            )}
          </div>
        )}
    </Modal>
  )
}
