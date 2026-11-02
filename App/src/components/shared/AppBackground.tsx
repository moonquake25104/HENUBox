/** 极光环境背景层：固定铺满视口，置于内容之下，为毛玻璃面板提供景深。 */
export function AppBackground() {
  return <div className="app-aurora fixed inset-0 -z-10" aria-hidden="true" />
}
