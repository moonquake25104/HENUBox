import { useEffect, useState } from "react"
import { Server, Check } from "lucide-react"
import { apiOrigin, nativeApp } from "@/lib/mobile/platform"
import { changeNativeServer } from "@/lib/mobile/session"
import { userErrorMessage } from "@/lib/errors"

export function NativeServerAddress({ onSaved }: { onSaved: () => void }) {
  const [address, setAddress] = useState(apiOrigin)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [saved, setSaved] = useState(false)
  useEffect(() => {
    const changed = () => {
      setAddress(apiOrigin)
      setSaved(true)
    }
    window.addEventListener("xique:server-changed", changed)
    return () => window.removeEventListener("xique:server-changed", changed)
  }, [])
  if (!nativeApp) return null
  async function save() {
    setBusy(true)
    setError("")
    try {
      await changeNativeServer(address)
      setAddress(apiOrigin)
      setSaved(true)
      onSaved()
    } catch (err) {
      setError(userErrorMessage(err, "服务地址未能保存"))
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="xq-native-server" aria-label="App 服务地址">
      <h3>
        <Server size={16} />
        连接你的HENUBox 服务
      </h3>
      <p>填写电脑或服务器的 HTTPS 地址。电脑需开机，局域网测试时手机需能访问同一网络。</p>
      <label htmlFor="native-api-origin">服务地址</label>
      <input
        id="native-api-origin"
        type="url"
        autoCapitalize="none"
        autoComplete="off"
        spellCheck={false}
        placeholder="https://电脑地址:8443"
        value={address}
        disabled={busy}
        onChange={(e) => {
          setAddress(e.target.value)
          setSaved(false)
        }}
      />
      <button
        type="button"
        className="xq-button small"
        disabled={busy || !address.trim()}
        onClick={() => void save()}
      >
        {saved && <Check size={14} />}
        {busy ? "正在保存…" : "保存服务地址"}
      </button>
      <small>更换地址会退出当前账号。账号和密码使用HENUBox自己的账号。</small>
      {error && (
        <p role="alert" className="xq-form-error">
          {error}
        </p>
      )}
      {saved && <p role="status">地址已保存，请登录。此步骤尚未验证服务是否在线。</p>}
    </section>
  )
}
