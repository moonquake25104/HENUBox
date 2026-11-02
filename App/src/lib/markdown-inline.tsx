import { Fragment, type ReactNode } from "react"

/**
 * Markdown 子集的共享渲染原语：
 * - `renderInline`：`**加粗**` / `*斜体*` → React 节点
 * - `cleanLines`：按行 trim + 去空行（块归一化，多处复用）
 * - `renderLines`：多行文本以 `<br/>` 拼接渲染，供段落/标题正文复用
 *
 * 供 AiBriefCard 的分节渲染共用。
 */

/** 按行 trim 并丢弃空行。 */
export function cleanLines(lines: string[]): string[] {
  return lines.map((l) => l.trim()).filter(Boolean)
}

/** 若干行文本 → 以 `<br/>` 拼接的 React 节点数组（跨行换行）。 */
export function renderLines(lines: string[]): ReactNode[] {
  return lines.map((line, j) => (
    <Fragment key={j}>
      {j > 0 && <br />}
      {renderInline(line)}
    </Fragment>
  ))
}

/** 单行文本内联渲染：`**加粗**` / `*斜体*` → React 节点。 */
export function renderInline(text: string): ReactNode[] {
  // 先按 **加粗** 切分
  const boldParts = text.split(/\*\*(.+?)\*\*/g)
  return boldParts.map((part, i) => {
    if (i % 2 === 1) {
      return <strong key={i}>{part}</strong>
    }
    // 再按 *斜体* 切分（非加粗段内）
    const italicParts = part.split(/\*([^*]+)\*/g)
    return italicParts.map((sub, j) =>
      j % 2 === 1 ? <em key={`${i}-${j}`}>{sub}</em> : sub,
    )
  })
}