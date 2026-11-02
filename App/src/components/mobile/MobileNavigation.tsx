import { useEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type PointerEvent } from "react"

import { mobileNavigation, type MobilePage } from "@/lib/mobile/navigation"
import { useUiStyle } from "@/lib/mobile/ui-style"
import { useBackdropNavigation } from "@/hooks/use-backdrop-navigation"
const clamp = (n: number) => Math.min(3, Math.max(0, n))

export function MobileNavigation({ page, navigate, suspended }: { page: MobilePage; navigate: (page: MobilePage) => void; suspended: boolean }) {
  const root = useRef<HTMLElement>(null)
  const gesture = useRef<{ id: number; x: number; y: number; index: number; stride: number; moved: boolean; committed: MobilePage } | null>(null)
  const [position, setPosition] = useState<number | null>(null)
  const ignoreClick = useRef(0)
  const index = mobileNavigation.findIndex(n => n.id === page)
  const { style } = useUiStyle()
  useBackdropNavigation(root, style === "glass" && !suspended, {selected:index,position:position??index,pressed:position!==null})
  useEffect(() => {
    const cancel = () => { gesture.current = null; setPosition(null) }
    const hashChanged = () => { if (gesture.current && location.hash.slice(2) !== gesture.current.committed) cancel() }
    window.addEventListener("blur", cancel); window.addEventListener("hashchange", hashChanged)
    return () => { window.removeEventListener("blur", cancel); window.removeEventListener("hashchange", hashChanged) }
  }, [])
  function down(e: PointerEvent<HTMLElement>) {
    if (suspended || !e.isPrimary || e.button !== 0) return
    ignoreClick.current = 0
    const button = (e.target as HTMLElement).closest<HTMLButtonElement>("button[data-nav-index]")
    if (!button || !root.current) return
    gesture.current = { id: e.pointerId, x: e.clientX, y: e.clientY, index: Number(button.dataset.navIndex), stride: (root.current.clientWidth - 12) / 4, moved: false, committed: page }
  }
  function move(e: PointerEvent<HTMLElement>) {
    const g = gesture.current
    if (!g || g.id !== e.pointerId) return
    const dx = e.clientX - g.x, dy = e.clientY - g.y
    if (!g.moved) {
      if (Math.abs(dy) > 10 && Math.abs(dy) > Math.abs(dx)) { gesture.current = null; return }
      if (Math.abs(dx) < 7 || Math.abs(dx) <= Math.abs(dy)) return
      if (document.activeElement instanceof HTMLElement && root.current?.contains(document.activeElement)) document.activeElement.blur()
      g.moved = true; root.current?.setPointerCapture(e.pointerId)
    }
    setPosition(clamp(g.index + dx / g.stride))
  }
  function up(e: PointerEvent<HTMLElement>) {
    const g = gesture.current
    if (!g || g.id !== e.pointerId) return
    gesture.current = null
    if (g.moved) {
      ignoreClick.current = e.timeStamp + 500
      const target = Math.round(clamp(g.index + (e.clientX - g.x) / g.stride))
      setPosition(null); navigate(mobileNavigation[target].id)
      if (root.current?.hasPointerCapture(e.pointerId)) root.current.releasePointerCapture(e.pointerId)
    }
  }
  function cancel(e: PointerEvent<HTMLElement>) {
    if (gesture.current?.id !== e.pointerId) return
    gesture.current = null; ignoreClick.current = e.timeStamp + 500; setPosition(null)
  }
  function key(e: KeyboardEvent<HTMLButtonElement>, at: number) {
    const target = e.key === "ArrowRight" ? (at + 1) % 4 : e.key === "ArrowLeft" ? (at + 3) % 4 : e.key === "Home" ? 0 : e.key === "End" ? 3 : -1
    if (target < 0) return
    e.preventDefault(); navigate(mobileNavigation[target].id)
    root.current?.querySelector<HTMLButtonElement>(`button[data-nav-index="${target}"]`)?.focus()
  }
  return <nav ref={root} inert={suspended} aria-hidden={suspended || undefined} className="xm-navigation" aria-label="手机主导航" data-renderer="web" data-dragging={position !== null || undefined}
    style={{ "--nav-position": position ?? index } as CSSProperties} onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={cancel}
    onLostPointerCapture={e => { if (!root.current?.hasPointerCapture(e.pointerId)) cancel(e) }}>
    <span className="xm-nav-indicator" aria-hidden="true" />
    {mobileNavigation.map(({ id, title, icon: Icon }, i) => <button key={id} data-nav-index={i} className={Math.round(position ?? index) === i ? "selected" : ""} aria-label={title} aria-current={page === id ? "page" : undefined}
      onKeyDown={e => key(e, i)} onClick={e => { if (e.detail === 0 || e.timeStamp >= ignoreClick.current) navigate(id) }}>
      <Icon size={23} aria-hidden="true" /><span className="xm-nav-label" aria-hidden="true">{title}</span>
    </button>)}
  </nav>
}
