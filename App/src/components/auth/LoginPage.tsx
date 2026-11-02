import { useEffect, useState, type FormEvent } from "react"
import {
  ArrowRight,
  BookOpen,
  LockKeyhole,
  UserRound,
  CalendarDays,
  Eye,
  EyeOff,
} from "lucide-react"
import { login } from "@/lib/api/auth"
import { setToken } from "@/lib/api/client"
import { userErrorMessage } from "@/lib/errors"
import logo from "@/assets/xique.svg"
import { nativeApp, apiOrigin } from "@/lib/mobile/platform"
import { NativeServerAddress } from "./NativeServerAddress"

export function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [visible, setVisible] = useState(false)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const [serverReady, setServerReady] = useState(!nativeApp || Boolean(apiOrigin))
  useEffect(() => {
    const changed = () => {
      setPassword("")
      setError("")
      setServerReady(Boolean(apiOrigin))
    }
    window.addEventListener("xique:server-changed", changed)
    return () => window.removeEventListener("xique:server-changed", changed)
  }, [])
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy) return
    setBusy(true)
    setError("")
    try {
      const result = await login(username.trim(), password)
      await setToken(result.token, result.user.username)
      onSuccess()
    } catch (err) {
      setError(userErrorMessage(err, "登录失败，请检查账号和密码"))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="xq-login-page">
      <div className="xq-login-story">
        <div className="xq-brand">
          <img src={logo} alt="" />
          <span>
            <strong>HENUBox</strong>
            <small>河小盒</small>
          </span>
        </div>
        <div className="xq-login-message">
          <span className="xq-eyebrow">A LITTLE ORDER. A LIGHTER DAY.</span>
          <h1>
            校园生活，
            <br />
            从容有序<span>。</span>
          </h1>
          <p>
            把课程安排好，把时间留给热爱。
            <br />
            属于你的每一天，都值得认真对待。
          </p>
          <div className="xq-login-illustration" aria-hidden="true">
            <div className="xq-login-calendar">
              <div>
                <CalendarDays size={22} />
                <span>新的一天，安排妥当</span>
              </div>
              <i />
              <i />
              <i />
            </div>
            <img src={logo} alt="" />
            <span className="xq-login-float">
              <BookOpen size={17} />
              让每一节课都心中有数
            </span>
          </div>
        </div>
        <small>你的个人教务工作台</small>
      </div>
      <main className="xq-login-main">
        <form className="xq-login-form" onSubmit={submit}>
          <span className="xq-login-wave">欢迎回来</span>
          <h2>开启今天的好安排</h2>
          <p>登录HENUBox，继续你的校园日常。</p>
          {nativeApp && (
            <NativeServerAddress
              onSaved={() => {
                setServerReady(Boolean(apiOrigin))
                setPassword("")
                setError("")
              }}
            />
          )}
          <div className="xq-field">
            <label htmlFor="login-username">账号</label>
            <div className="xq-input-icon">
              <UserRound size={17} />
              <input
                id="login-username"
                required
                autoFocus
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder="输入你的账号"
              />
            </div>
          </div>
          <div className="xq-field">
            <label htmlFor="login-password">密码</label>
            <div className="xq-input-icon">
              <LockKeyhole size={17} />
              <input
                id="login-password"
                required
                type={visible ? "text" : "password"}
                autoComplete="current-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="输入你的密码"
              />
              <button
                type="button"
                onClick={() => setVisible((value) => !value)}
                aria-label={visible ? "隐藏密码" : "显示密码"}
              >
                {visible ? <EyeOff size={17} /> : <Eye size={17} />}
              </button>
            </div>
          </div>
          {error && (
            <p role="alert" className="xq-form-error">
              {error}
            </p>
          )}
          <button
            type="submit"
            className="xq-button primary xq-login-submit"
            disabled={busy || !username.trim() || !serverReady}
          >
            {busy ? "正在登录…" : "登录我的空间"}
            <ArrowRight size={17} />
          </button>
          <div className="xq-login-privacy">
            <LockKeyhole size={13} />
            使用HENUBox账号登录，与学校教务账号独立。
          </div>
        </form>
        <footer>HENUBox · 好好上课，也好好生活。</footer>
      </main>
    </div>
  )
}
