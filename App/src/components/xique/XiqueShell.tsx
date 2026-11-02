import type { ReactNode } from "react"
import { BookOpen, House, Settings2, Moon, Sun, ArrowUpRight, Feather } from "lucide-react"
import { useTheme } from "@/hooks/use-theme"
import logo from "@/assets/xique.svg"
import type { Page } from "@/App"

const navigation = [
  { id: "today" as const, label: "今日概览", short: "今天", icon: House },
  { id: "courses" as const, label: "我的课表", short: "课表", icon: BookOpen },
  { id: "settings" as const, label: "偏好设置", short: "设置", icon: Settings2 },
]
export function XiqueShell({
  page,
  navigate,
  connected,
  localSaved = false,
  children,
}: {
  page: Page
  navigate: (page: Page) => void
  connected: boolean
  localSaved?: boolean
  children: ReactNode
}) {
  const { theme, toggle } = useTheme()
  const current = navigation.find((item) => item.id === page)!
  return (
    <div className="xq-app">
      <a
        className="xq-skip"
        href="#main-content"
        onClick={(event) => {
          event.preventDefault()
          document.getElementById("main-content")?.focus()
        }}
      >
        跳到主要内容
      </a>
      <aside className="xq-sidebar">
        <button className="xq-brand" onClick={() => navigate("today")} aria-label="HENUBox首页">
          <img src={logo} alt="" />
          <span>
            <strong>HENUBox</strong>
            <small>河小盒</small>
          </span>
        </button>
        <div className="xq-nav-label">我的校园生活</div>
        <nav aria-label="主导航">
          {navigation.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              className={`xq-nav-item ${page === id ? "active" : ""}`}
              onClick={() => navigate(id)}
              aria-current={page === id ? "page" : undefined}
            >
              <Icon size={19} />
              <span>{label}</span>
              {page === id && <span className="xq-nav-dot" />}
            </button>
          ))}
        </nav>
        <div className="xq-sidebar-note">
          <Feather size={23} />
          <p>
            好好上课，
            <br />
            也好好生活。
          </p>
          <small>把时间留给重要的事</small>
        </div>
        <div className="xq-sidebar-bottom">
          <span className={`xq-connection ${connected ? "connected" : ""}`}>
            <i />
            {connected ? "数据服务已连接" : localSaved ? "本机课表可用" : "正在连接数据服务"}
          </span>
          <small>HENUBox · 个人工作台</small>
        </div>
      </aside>
      <div className="xq-workspace">
        <header className="xq-topbar">
          <div className="xq-mobile-brand">
            <img src={logo} alt="" />
            <strong>HENUBox</strong>
          </div>
          <div className="xq-breadcrumb">
            我的空间 <span>/</span> <strong>{current.label}</strong>
          </div>
          <div className="xq-top-actions">
            <span className="xq-private-tag">个人空间</span>
            <button
              className="xq-icon-button"
              onClick={toggle}
              aria-label={theme === "dark" ? "切换浅色模式" : "切换深色模式"}
            >
              {theme === "dark" ? <Sun size={19} /> : <Moon size={19} />}
            </button>
            <span className="xq-avatar" aria-label="个人账号">
              雀
            </span>
          </div>
        </header>
        <main id="main-content" tabIndex={-1} className="xq-main" key={page}>
          <div className="xq-content">
            {children}
            <footer className="xq-page-footer">
              <span>HENUBox，让校园生活更从容。</span>
              <button onClick={() => navigate("settings")}>
                我的偏好 <ArrowUpRight size={13} />
              </button>
            </footer>
          </div>
        </main>
      </div>
      <nav className="xq-bottom-nav" aria-label="手机导航">
        {navigation.map(({ id, short, icon: Icon }) => (
          <button
            key={id}
            className={page === id ? "active" : ""}
            aria-current={page === id ? "page" : undefined}
            onClick={() => navigate(id)}
          >
            <Icon size={22} />
            <span>{short}</span>
          </button>
        ))}
      </nav>
    </div>
  )
}
