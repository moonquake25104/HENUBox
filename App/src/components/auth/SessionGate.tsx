import { LoaderCircle, RefreshCw } from "lucide-react"
import logo from "@/assets/xique.svg"

export function SessionGate({
  error,
  retry,
  reset,
}: {
  error: string
  retry: () => void
  reset: () => void
}) {
  return (
    <main className="xq-session-gate">
      <img src={logo} alt="HENUBox" />
      <h1>{error ? "本机登录暂时无法恢复" : "正在打开你的空间"}</h1>
      <p role={error ? "alert" : "status"}>{error || "正在读取安全登录信息…"}</p>
      {error ? (
        <div>
          <button className="xq-button primary" onClick={retry}>
            <RefreshCw size={15} />
            重新读取
          </button>
          <button className="xq-button" onClick={reset}>
            清除本机登录
          </button>
        </div>
      ) : (
        <LoaderCircle className="animate-spin" size={24} />
      )}
    </main>
  )
}
