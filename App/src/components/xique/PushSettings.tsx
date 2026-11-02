import { useEffect, useState } from "react"
import { BellRing, RefreshCw } from "lucide-react"
import {
  pushConfig,
  pushDevice,
  pushTests,
  savePushDevice,
  sendPushTest,
  confirmPushTest,
  storedPushDevice,
  rememberPushDevice,
  detachPushDevice,
  type PushConfig,
  type PushTest,
} from "@/lib/api/push"
import { userErrorMessage } from "@/lib/errors"

const states: Record<string, string> = {
  queued: "等待发送",
  sending: "正在发送",
  accepted: "服务已接受 · 待确认",
  failed: "发送失败",
  expired: "已过期",
  unknown: "结果不确定",
  cancelled: "已取消",
}
function capability() {
  if (!window.isSecureContext)
    return "请使用手机可访问、证书受信任的 HTTPS 地址。普通局域网 HTTP 地址无法启用推送。"
  if (!("serviceWorker" in navigator) || !("PushManager" in window) || !("Notification" in window))
    return "当前浏览器未提供所需推送能力，请记录浏览器版本。"
  return ""
}
function publicKey(value: string): Uint8Array<ArrayBuffer> {
  const raw = atob(
    value.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (value.length % 4)) % 4),
  )
  return Uint8Array.from(raw, (char) => char.charCodeAt(0))
}
async function timed<T>(work: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    return await Promise.race([
      work,
      new Promise<never>((_, reject) => {
        timer = setTimeout(
          () => reject(new Error("浏览器推送注册超过 30 秒，请检查网络后重新检查设备订阅。")),
          30000,
        )
      }),
    ])
  } finally {
    clearTimeout(timer)
  }
}
export function PushSettings() {
  const [config, setConfig] = useState<PushConfig | null>(null)
  const [device, setDevice] = useState<number | null>(null)
  const [tests, setTests] = useState<PushTest[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [message, setMessage] = useState("")
  const [permission, setPermission] = useState(
    "Notification" in window ? Notification.permission : "unsupported",
  )
  const unsupported = capability()
  async function refresh() {
    const [cfg, jobs] = await Promise.all([pushConfig(), pushTests()])
    setConfig(cfg)
    setTests(jobs)
    const id = storedPushDevice()
    if (id) {
      const current = await pushDevice(id).catch(() => null)
      setDevice(current?.active ? id : null)
    } else setDevice(null)
    if ("Notification" in window) setPermission(Notification.permission)
  }
  useEffect(() => {
    let live = true
    const update = () => {
      if (live)
        void refresh().catch((err) => {
          if (live) setError(userErrorMessage(err, "手机通知状态读取失败"))
        })
    }
    update()
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") update()
    }, 5000)
    window.addEventListener("focus", update)
    return () => {
      live = false
      clearInterval(timer)
      window.removeEventListener("focus", update)
    }
  }, [])
  async function action(work: () => Promise<void>) {
    setBusy(true)
    setError("")
    setMessage("")
    try {
      await work()
      await refresh()
    } catch (err) {
      setError(userErrorMessage(err, "手机通知操作失败"))
    } finally {
      setBusy(false)
    }
  }
  async function enable() {
    // Permission request stays in the direct user gesture, before network awaits.
    const granted = await Notification.requestPermission()
    setPermission(granted)
    if (granted !== "granted")
      throw new Error("通知权限未开启。请在 Edge 的网站权限及系统通知设置中检查。")
    const registration = await navigator.serviceWorker.register("/push-sw.js", { scope: "/" })
    await timed(navigator.serviceWorker.ready)
    let subscription = await registration.pushManager.getSubscription()
    const key = publicKey(config!.public_key)
    const oldKey = subscription?.options.applicationServerKey
    if (
      subscription &&
      oldKey &&
      Array.from(new Uint8Array(oldKey)).join() !== Array.from(key).join()
    ) {
      await detachPushDevice()
      subscription = null
    }
    if (!subscription)
      subscription = await timed(
        registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: key,
        }),
      )
    const saved = await savePushDevice(subscription.toJSON())
    rememberPushDevice(saved.id)
    setDevice(saved.id)
    setMessage("设备已启用。请发送测试通知，确认手机通知栏实际收到。")
  }
  async function test(delay: number) {
    await sendPushTest(device!, delay)
    setMessage(
      delay
        ? `测试已安排在 ${delay / 60} 分钟后。请关闭网页并锁屏，电脑需保持运行。`
        : "测试已排队。请检查手机通知栏；服务接受并不代表手机已收到。",
    )
  }
  return (
    <section className="xq-panel xq-settings-section" aria-labelledby="push-heading">
      <div className="xq-section-heading">
        <div className="xq-setting-title">
          <span className="xq-stat-icon blue">
            <BellRing size={21} />
          </span>
          <div>
            <h2 id="push-heading">手机通知测试</h2>
            <p>先确认手机收到，再接入上课提醒。</p>
          </div>
        </div>
        <span className="xq-status-pill neutral">{device ? "设备已启用" : "待启用"}</span>
      </div>
      <p className="xq-info-note">
        目标设备：vivo X200s ·
        Edge。当前只发送你主动安排的测试通知，尚未自动发送课程提醒。电脑和后端需持续运行。
      </p>
      <p>
        浏览器权限：
        {
          (
            {
              granted: "已允许",
              denied: "已拒绝",
              default: "尚未选择",
              unsupported: "不支持",
            } as Record<string, string>
          )[permission]
        }
      </p>
      {unsupported && <p className="xq-info-note">{unsupported}</p>}
      {message && (
        <p className="xq-success" role="status">
          {message}
        </p>
      )}
      {error && (
        <p className="xq-error" role="alert">
          {error}
        </p>
      )}
      <div className="xq-push-actions">
        <button
          className="xq-button primary small"
          disabled={busy || !!unsupported || !config?.enabled}
          onClick={() => void action(enable)}
        >
          {device ? "重新检查设备订阅" : "启用手机提醒"}
        </button>
        {device && (
          <>
            <button
              className="xq-button small"
              disabled={busy}
              onClick={() => void action(() => test(0))}
            >
              立即测试
            </button>
            <button
              className="xq-button small"
              disabled={busy}
              onClick={() => void action(() => test(60))}
            >
              1 分钟后测试
            </button>
            <button
              className="xq-button small"
              disabled={busy}
              onClick={() => void action(() => test(900))}
            >
              15 分钟后测试
            </button>
            <button
              className="xq-button small"
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  await detachPushDevice()
                  setDevice(null)
                  setMessage("当前设备已停用，待发送测试已取消。")
                })
              }
            >
              停用此设备
            </button>
          </>
        )}
        <button
          className="xq-button small"
          disabled={busy}
          onClick={() => void action(async () => {})}
        >
          <RefreshCw size={14} />
          刷新状态
        </button>
      </div>
      <div className="xq-push-history" aria-live="polite">
        {tests.length === 0 ? (
          <p>还没有测试记录。</p>
        ) : (
          tests.map((test) => (
            <div className="xq-push-record" key={test.id}>
              <div>
                <strong>
                  测试 #{test.id} ·{" "}
                  {test.confirmed ? "你已确认收到" : states[test.status] || test.status}
                </strong>
                <p>
                  {new Date(test.due_at).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" })}
                  （北京时间）
                </p>
                <p>{test.detail}</p>
              </div>
              {!test.confirmed && ["accepted", "unknown"].includes(test.status) && (
                <button
                  className="xq-button small"
                  disabled={busy}
                  onClick={() =>
                    void action(async () => {
                      await confirmPushTest(test.id)
                    })
                  }
                >
                  我已收到 #{test.id}
                </button>
              )}
            </div>
          ))
        )}
      </div>
    </section>
  )
}
