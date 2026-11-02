import { useEffect, useLayoutEffect, useRef, type RefObject } from "react"
import { Capacitor, registerPlugin } from "@capacitor/core"

interface Backdrop {
  getCapabilities(): Promise<{supported: boolean}>
  configure(value: {x: number; y: number; width: number; height: number; viewport_width: number; selected: number; position: number; pressed: boolean; dark: boolean; ink: string; reduced: boolean; sequence: number}): Promise<{active: boolean}>
  hide(value: {sequence: number}): Promise<void>
}
const bridge = registerPlugin<Backdrop>("XiqueBackdrop")
type Selection = {selected: number; position: number; pressed: boolean}

/** Upstream Compose Backdrop paints only visuals; existing DOM owns all input. */
export function useBackdropNavigation(ref: RefObject<HTMLElement | null>, enabled: boolean, {selected,position,pressed}: Selection) {
  const latest = useRef({selected,position,pressed}), queueRef = useRef<() => void>(() => {}), sequence = useRef(0)
  useLayoutEffect(() => { latest.current = {selected,position,pressed}; queueRef.current() }, [selected,position,pressed])
  useEffect(() => {
    const root = ref.current
    if (!root || !enabled || !Capacitor.isPluginAvailable("XiqueBackdrop")) return
    if (!sequence.current) sequence.current = Date.now() * 1000
    let alive = true, ready = false, frame = 0, busy = false, dirty = false
    const transparent = matchMedia("(prefers-reduced-transparency: reduce)"), contrast = matchMedia("(prefers-contrast: more)"), calm = matchMedia("(prefers-reduced-motion: reduce)")
    const hide = () => { delete root.dataset.backdropVisual; return bridge.hide({sequence: ++sequence.current}).catch(() => {}) }
    const update = async () => {
      frame = 0
      if (!alive || !ready) return
      if (busy) { dirty = true; return }
      if (document.hidden || transparent.matches || contrast.matches) { await hide(); return }
      const r = root.getBoundingClientRect(), rgb = getComputedStyle(root).color.match(/\d+/g)
      const ink = rgb ? "#" + rgb.slice(0,3).map(v => Number(v).toString(16).padStart(2,"0")).join("") : "#243b43"
      busy = true; dirty = false
      // Hide native-source icons before recording. Buttons keep their DOM labels/hit areas.
      root.dataset.backdropVisual = "active"
      try {
        const result = await bridge.configure({x:r.x,y:r.y,width:r.width,height:r.height,viewport_width:innerWidth,...latest.current,dark:document.documentElement.classList.contains("dark"),ink,reduced:calm.matches,sequence:++sequence.current})
        if (alive && !result.active) await hide()
      } catch { if (alive) await hide() }
      finally { busy = false; if (alive && dirty) queue() }
    }
    const queue = () => { if (!frame && alive) frame = requestAnimationFrame(() => void update()) }
    queueRef.current = queue
    void bridge.getCapabilities().then(result => { if (alive && result.supported) { ready = true; queue() } }).catch(() => {})
    const resize = new ResizeObserver(queue); resize.observe(root)
    const theme = new MutationObserver(queue);theme.observe(document.documentElement,{attributes:true,attributeFilter:["class","data-mobile-palette"]})
    window.addEventListener("resize",queue);document.addEventListener("visibilitychange",queue)
    for (const q of [transparent,contrast,calm]) q.addEventListener("change",queue)
    return () => { alive=false;queueRef.current=()=>{};cancelAnimationFrame(frame);resize.disconnect();theme.disconnect();window.removeEventListener("resize",queue);document.removeEventListener("visibilitychange",queue);for(const q of [transparent,contrast,calm])q.removeEventListener("change",queue);void hide() }
  }, [enabled, ref])
}
