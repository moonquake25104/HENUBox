import notices from "@/lib/mobile/open-source-notices.json"
import "./about-legal.css"
export function AboutLegal() {
  return <div className="xl-about">
    <section className="xp-group"><h2>感谢与你相遇</h2><div className="xp-card xl-copy"><p>感谢大家使用 HENUBox（河小盒）。这是一个面向学习交流的非官方校园工具，免费使用，无广告；不代表河南大学或相关服务提供方。</p><p>应用仍在持续完善，可能有不够周到的地方或 Bug，感谢理解与包容。也欢迎分享更好的想法，一起把日常的小事安排好。</p></div></section>
    <section className="xp-group"><h2>使用声明</h2><div className="xp-card xl-copy"><p>课表、空闲教室、校历、预约等信息可能因网络、学校系统调整或缓存而延迟、缺失或出错。重要事项请以学校和相关服务的官方通知为准，并自行核对。</p><p>图书馆、学校邮箱及部分校园服务在官方或第三方网页中操作；网页服务的账号规则、可用性和条款由其运营方决定。</p></div></section>
    <section className="xp-group"><h2>数据隐私</h2><div className="xp-card xl-copy"><h3>保存在手机的数据</h3><p>手机本地模式下，课表、个人修改、待办、专注记录、偏好与导入的背景图片、音频、视频保存在本机。当前没有为这些记录提供开发者云备份，不会将它们用于广告或出售。</p><h3>联网与学校授权</h3><p>登录学校、检查课表更新或查询教室时，应用会访问相应学校服务；打开校园官网、图书馆、邮箱或全景网页时，会访问其服务及网页依赖的第三方资源。服务方可以接收到访问所需的请求信息。</p><p>教务密码与验证码由你在学校网页输入，本应用不要求将它们交给开发者。用于后续查询的教务会话使用 Android Keystore 支持的加密保存；其他网页的 Cookie 和站点数据由对应 WebView 与网站处理。</p><h3>权限与删除</h3><p>通知、准时提醒、专注后台计时、勿扰及图片保存等授权按对应功能使用。首次提示不代替系统权限授权，拒绝某项权限可能影响该项功能。</p><p>可以编辑或删除应用内相应记录，或在学校连接页面断开教务连接。如需清除全部本机数据，可在 Android 设置清除应用数据或卸载；这些操作会使本机记录丢失，请先核对需要保留的内容。</p></div></section>
    <section className="xp-group"><h2>安全与反馈</h2><div className="xp-card xl-copy"><p>本版本采取本地存储、HTTPS 访问、教务会话加密等措施，未接入开发者广告或行为统计 SDK。软件尚未经过独立安全审计，不能承诺绝对安全、没有缺陷或数据永不丢失。</p><p>欢迎反馈 Bug 和建议。反馈时请隐藏姓名、学号、密码、验证码、会话信息和其他个人资料；不要公开未脱敏的课表或网络记录。</p><div className="xl-repository"><strong>GitHub 仓库</strong><a href="https://github.com/moonquake25104/HENUBox" target="_blank" rel="noreferrer">moonquake25104/HENUBox</a></div><p>源码、版本发布与 Issues 反馈入口以仓库页面为准。</p></div></section>
    <section className="xp-group"><h2>开源许可与致谢</h2><div className="xp-card xl-copy"><p>项目沿用现有 MIT 许可证，并保留 Kairos 的原始版权与许可声明，以及维护者 moonquake25104 的版权。第三方组件分别遵循其许可证，代码许可不自动授权第三方校徽、官网图文或其他图片素材。</p><p>感谢 Kairos、React、Capacitor、Radix UI、Lucide、AndroidX/Compose、Kotlin、Go、AndroidLiquidGlass/Backdrop、字体与音频作者，以及参与测试和分享建议的朋友。</p><p>交互设计曾参考 ZZUPocket、Goodtime、Tomato 等项目；参考不等同于将它们的完整代码或素材并入应用。</p><div className="xl-notices">{notices.map((n, i) => <details key={i}><summary><span>{n.name}<small>{n.version} · {n.license}</small></span></summary><p className="xl-source">来源：<span>{n.url}</span></p><pre>{n.text}</pre></details>)}</div></div></section>
    <p className="xp-footer">说明日期：2026-11-02 · 谢谢你的使用与理解。</p>
  </div>
}
