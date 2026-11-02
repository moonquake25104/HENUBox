import { useState } from "react"
import { AlertCircle, CheckCircle2, ExternalLink, RefreshCw } from "lucide-react"
import { checkForUpdate, openReleasePage, type UpdateResult } from "@/lib/mobile/update"
import "./update-checker.css"

type State =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "current"; result: UpdateResult }
  | { kind: "available"; result: UpdateResult }
  | { kind: "error"; message: string }

function dateText(value: string): string {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return date.toLocaleDateString("zh-CN", { year: "numeric", month: "long", day: "numeric" })
}

export function UpdateChecker() {
  const [state, setState] = useState<State>({ kind: "idle" })
  const checking = state.kind === "checking"
  const result = state.kind === "current" || state.kind === "available" ? state.result : null

  const check = async () => {
    if (checking) return
    setState({ kind: "checking" })
    try {
      const next = await checkForUpdate()
      setState(next.updateAvailable ? { kind: "available", result: next } : { kind: "current", result: next })
    } catch (error) {
      setState({
        kind: "error",
        message: error instanceof Error ? error.message : "暂时无法检查更新，请稍后重试。",
      })
    }
  }

  const open = (url: string) => {
    void openReleasePage(url).catch(() => {
      setState({ kind: "error", message: "无法打开浏览器，请手动访问 GitHub Releases。" })
    })
  }

  return (
    <section className="xp-group">
      <h2>版本更新</h2>
      <div className="xp-card xl-copy uc-card">
        <div className="uc-version">
          <span>当前版本</span>
          <strong>v{result?.currentVersion ?? __APP_VERSION__}</strong>
        </div>

        {state.kind === "idle" && (
          <p className="uc-hint">从 GitHub Releases 检查最新版本；不会自动下载或安装。</p>
        )}
        {state.kind === "checking" && (
          <p className="uc-status" role="status"><RefreshCw size={15} className="animate-spin" />正在检查更新…</p>
        )}
        {state.kind === "current" && (
          <p className="uc-status is-ok" role="status">
            <CheckCircle2 size={15} />已是最新版本{result ? `（v${result.latestVersion}）` : ""}
          </p>
        )}
        {state.kind === "available" && result && (
          <div className="uc-update">
            <p className="uc-status is-new"><ExternalLink size={15} />发现新版本 v{result.latestVersion}</p>
            {result.publishedAt && <p className="uc-hint">发布于 {dateText(result.publishedAt)}</p>}
            {result.notes && <pre className="uc-notes">{result.notes}</pre>}
            <button className="xp-row uc-action" onClick={() => open(result.releaseUrl)}>
              <span className="xp-row-icon"><ExternalLink size={19} /></span>
              <span className="xp-row-copy"><strong>前往 GitHub 查看并下载</strong><small>在浏览器中打开 Release 页面</small></span>
              <ExternalLink size={16} />
            </button>
          </div>
        )}
        {state.kind === "error" && (
          <div className="uc-error" role="alert">
            <p className="uc-status is-error"><AlertCircle size={15} />{state.message}</p>
            <button className="uc-link" onClick={() => open("https://github.com/moonquake25104/HENUBox/releases/latest")}>
              手动打开发布页
            </button>
          </div>
        )}

        <button className="xp-row uc-check" disabled={checking} onClick={() => void check()}>
          <span className="xp-row-icon"><RefreshCw size={19} className={checking ? "animate-spin" : undefined} /></span>
          <span className="xp-row-copy">
            <strong>{checking ? "正在检查…" : "检查更新"}</strong>
            <small>只访问 GitHub，不会上传课表或账号信息</small>
          </span>
          <ExternalLink size={16} />
        </button>
      </div>
    </section>
  )
}
