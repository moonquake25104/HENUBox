import { Modal } from "@/components/shared/modal"
import type { useIntroNotice } from "@/hooks/use-intro-notice"
import "./about-legal.css"
export function FirstRunNotice({ notice }: { notice: ReturnType<typeof useIntroNotice> }) {
  if (!notice.open) return null
  return <Modal open title="欢迎使用 HENUBox" description="河小盒 · 一份简短的使用说明" className="xm-intro-modal" layer={110} onOpenChange={open => { if (!open) void notice.acknowledge() }}>
    <div className="xl-intro-copy"><p>感谢使用！这是一款非官方、免费无广告的校园学习交流工具。</p><p>课表、待办和专注记录默认保存在本机；连接学校或打开第三方网页时，会访问相应服务。</p><p>应用仍在完善，可能存在错误。教务信息以学校官方为准，欢迎反馈问题和建议；反馈前请隐藏个人信息。</p><p>完整使用声明、数据隐私和开源致谢，详见<strong>「我的 → 关于HENUBox」</strong>。</p></div>
    {notice.error && <p role="alert" className="xl-error">{notice.error}</p>}
    <button className="xm-primary xl-confirm" disabled={notice.busy} onClick={() => void notice.acknowledge()}>{notice.busy ? "正在保存…" : "我已了解，开始使用"}</button>
  </Modal>
}
