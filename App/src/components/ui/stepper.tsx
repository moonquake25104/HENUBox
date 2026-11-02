import { useRef, useCallback } from "react"
import { Minus, Plus } from "lucide-react"
import { cn } from "@/lib/utils"

interface StepperProps {
  value: number
  onChange: (value: number) => void
  min: number
  max: number
  step?: number
  className?: string
}

export function Stepper({ value, onChange, min, max, step = 1, className }: StepperProps) {
  const holdRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const holdDirRef = useRef<1 | -1>(1)

  const adjust = useCallback(
    (dir: 1 | -1) => {
      const next = value + dir * step
      if (next >= min && next <= max) onChange(next)
    },
    [value, step, min, max, onChange],
  )

  const startHold = useCallback(
    (dir: 1 | -1) => {
      holdDirRef.current = dir
      adjust(dir)
      if (holdRef.current) clearInterval(holdRef.current)
      holdRef.current = setInterval(() => adjust(holdDirRef.current), 120)
    },
    [adjust],
  )

  const stopHold = useCallback(() => {
    if (holdRef.current) {
      clearInterval(holdRef.current)
      holdRef.current = null
    }
  }, [])

  const atMin = value <= min
  const atMax = value >= max

  return (
    <div className={cn("flex items-center gap-1", className)}>
      <button
        type="button"
        disabled={atMin}
        onPointerDown={() => startHold(-1)}
        onPointerUp={stopHold}
        onPointerLeave={stopHold}
        className={cn(
          "flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-input transition-colors",
          "min-h-11 min-w-11 md:min-h-0 md:min-w-0",
          atMin
            ? "opacity-30 cursor-not-allowed"
            : "hover:bg-muted active:bg-muted/60",
        )}
        aria-label="减少"
      >
        <Minus className="h-3.5 w-3.5" />
      </button>
      <span className="flex h-9 min-w-[2.5rem] items-center justify-center rounded-md bg-muted/40 px-2 text-sm font-medium tabular-nums">
        {value}
      </span>
      <button
        type="button"
        disabled={atMax}
        onPointerDown={() => startHold(1)}
        onPointerUp={stopHold}
        onPointerLeave={stopHold}
        className={cn(
          "flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-input transition-colors",
          "min-h-11 min-w-11 md:min-h-0 md:min-w-0",
          atMax
            ? "opacity-30 cursor-not-allowed"
            : "hover:bg-muted active:bg-muted/60",
        )}
        aria-label="增加"
      >
        <Plus className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}
