import { useEffect, useLayoutEffect, useRef, type RefObject } from "react"

/** One short, interruptible animation on the live view. No cloned/outgoing pages. */
export function useContentTransition<T extends HTMLElement>(
  ref: RefObject<T | null>, key: string, position = 0, enabled = true, enter = false,
) {
  const previous = useRef<{ key: string; position: number; enabled: boolean } | null>(null)
  const active = useRef<Animation | null>(null)
  useLayoutEffect(() => {
    const target = ref.current, before = previous.current
    previous.current = { key, position, enabled }
    active.current?.cancel(); active.current = null
    if (!target || !enabled || document.hidden || matchMedia("(prefers-reduced-motion: reduce)").matches || !target.animate) return
    if (!before && !enter || before?.key === key && before.enabled) return
    const first = !before, back = !!before && (position < before.position || !before.enabled)
    const animation = target.animate([
      { opacity: .78, transform: first ? "translateY(10px)" : `translateX(${back ? -10 : 10}px)` },
      { opacity: 1, transform: "none" },
    ], { duration: 180, easing: "cubic-bezier(.2,.7,.2,1)", fill: "none" })
    active.current = animation
    void animation.finished.then(() => { if (active.current === animation) { active.current = null; animation.cancel() } }, () => {})
    return () => { animation.cancel(); if (active.current === animation) active.current = null }
  }, [ref, key, position, enabled, enter])
  useEffect(() => {
    const preference = matchMedia("(prefers-reduced-motion: reduce)")
    const cancel = () => { if (document.hidden || preference.matches) { active.current?.cancel(); active.current = null } }
    preference.addEventListener("change", cancel); document.addEventListener("visibilitychange", cancel)
    return () => { preference.removeEventListener("change", cancel); document.removeEventListener("visibilitychange", cancel); active.current?.cancel() }
  }, [])
}
