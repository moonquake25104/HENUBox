import { useEffect, useState } from "react"
import { BellRing, Download, Copy } from "lucide-react"
import { apiFetch } from "@/lib/api/client"
import { userErrorMessage } from "@/lib/errors"

interface Config {
  enabled: boolean
  server_url: string
  topic: string
  username: string
  apk_url: string
  course_reminders: boolean
}
interface Receipt {
  id: string
  time: number
  delayed: boolean
  status: string
}
export function NtfySettings() {
  const [config, setConfig] = useState<Config | null>(null)
  const [password, setPassword] = useState("")
  const [subscribed, setSubscribed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [message, setMessage] = useState("")
  const [receipt, setReceipt] = useState<Receipt | null>(null)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    let live = true
    apiFetch<Config>("/api/ntfy/config")
      .then((value) => {
        if (live) {
          setConfig(value)
          setError("")
        }
      })
      .catch((err) => {
        if (live) setError(userErrorMessage(err, "接收服务读取失败"))
      })
    return () => {
      live = false
    }
  }, [retry])
  async function run(work: () => Promise<void>) {
    setBusy(true)
    setError("")
    setMessage("")
    try {
      await work()
    } catch (err) {
      setError(userErrorMessage(err, "通知操作失败"))
    } finally {
      setBusy(false)
    }
  }
  async function reveal() {
    if (password) {
      setPassword("")
      return
    }
    const data = await apiFetch<{ password: string }>("/api/ntfy/setup", { method: "POST" })
    setPassword(data.password)
  }
  async function send(delay: number) {
    const data = await apiFetch<Receipt>("/api/ntfy/test", {
      method: "POST",
      body: JSON.stringify({ delay_seconds: delay }),
    })
    setReceipt(data)
    setMessage(
      delay
        ? `已安排 ${delay / 60} 分钟后的测试。请关闭网页并锁屏，保持电脑运行。`
        : "通知服务已接受。请检查 ntfy 的手机通知，收到后在聊天中告诉我。",
    )
  }
  const field = (label: string, value: string) => (
    <div className="xq-field" key={label}>
      <label>{label}</label>
      <div className="xq-native-value">
        <code>{value}</code>
        <button
          className="xq-button small"
          aria-label={`复制${label}`}
          disabled={busy}
          onClick={() =>
            void run(async () => {
              await navigator.clipboard.writeText(value)
              setMessage(`${label}已复制`)
            })
          }
        >
          <Copy size={14} />
        </button>
      </div>
    </div>
  )
  return (
    <section className="xq-panel xq-settings-section" aria-labelledby="ntfy-heading">
      <div className="xq-section-heading">
        <div className="xq-setting-title">
          <span className="xq-stat-icon blue">
            <BellRing size={21} />
          </span>
          <div>
            <h2 id="ntfy-heading">Android 通知接收</h2>
            <p>使用自建 ntfy 服务，直接连接你的手机。</p>
          </div>
        </div>
        <span className="xq-status-pill neutral">{config ? "服务已配置" : "读取配置中"}</span>
      </div>
      <p className="xq-info-note">
        课表继续使用 Edge。通知由 ntfy Android 应用接收，此通道不使用 Google
        推送。当前先验证测试通知，上课自动提醒尚未接入。
      </p>
      {error && (
        <div className="xq-error" role="alert">
          <p>{error}</p>
          {!config && (
            <button className="xq-button small" onClick={() => setRetry((x) => x + 1)}>
              重新读取接收服务
            </button>
          )}
        </div>
      )}
      {message && (
        <p className="xq-success" role="status">
          {message}
        </p>
      )}
      {config && (
        <>
          <ol className="xq-native-steps">
            <li>下载安装官方 ntfy 接收应用，并允许通知。</li>
            <li>
              添加下面的服务器账号，再订阅指定主题。若提示证书不可信，先核对并信任本项目测试证书。
            </li>
            <li>允许 ntfy 后台运行，完成订阅后再发送测试。</li>
          </ol>
          <a className="xq-button small" href={config.apk_url} download>
            <Download size={15} />
            下载 ntfy 安装包
          </a>
          <div className="xq-native-fields">
            {field("服务器地址", config.server_url)}
            {field("订阅主题", config.topic)}
            {field("接收用户名", config.username)}
            {password && field("接收密码", password)}
          </div>
          <button className="xq-button small" disabled={busy} onClick={() => void run(reveal)}>
            {password ? "隐藏接收密码" : "显示接收密码"}
          </button>
          <p className="xq-info-note">
            接收账号只能读取此主题。网页退出不会停用 ntfy；要停止接收，请在 ntfy 中删除该主题订阅。
          </p>
          <label className="xq-native-confirm">
            <input
              type="checkbox"
              checked={subscribed}
              onChange={(e) => setSubscribed(e.target.checked)}
            />
            我已在 ntfy 中完成账号配置和主题订阅
          </label>
          <div className="xq-push-actions">
            {[0, 60, 900].map((delay) => (
              <button
                className={`xq-button small ${delay === 0 ? "primary" : ""}`}
                key={delay}
                disabled={busy || !subscribed}
                onClick={() => void run(() => send(delay))}
              >
                {busy ? "正在处理…" : delay === 0 ? "发送 ntfy 测试" : `${delay / 60} 分钟后发送`}
              </button>
            ))}
          </div>
          <p>两次测试至少间隔 30 秒。锁屏测试时，保持电脑开机并连接手机热点。</p>
          {receipt && (
            <p className="xq-info-note">
              测试 {receipt.id} · 服务已接受，手机送达待你确认。
              {receipt.delayed
                ? `计划时间 ${new Date(receipt.time * 1000).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" })}（北京时间）`
                : ""}
            </p>
          )}
        </>
      )}
    </section>
  )
}
