import { useState, useRef, useEffect } from "react"
import { ChevronDown } from "lucide-react"
import { cn } from "@/lib/utils"

interface FilterOption {
  value: string
  label: string
}

interface FilterChipProps {
  label: string
  options: FilterOption[]
  value: string
  onChange: (value: string) => void
  className?: string
}

/** 胶囊筛选按钮：显示当前选中值 + 下拉图标，点击弹出浮层选项列表。 */
export function FilterChip({ label, options, value, onChange, className }: FilterChipProps) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  // Close on click outside
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    if (open) {
      document.addEventListener("mousedown", handleClickOutside)
      return () => document.removeEventListener("mousedown", handleClickOutside)
    }
  }, [open])

  const selectedOption = options.find((o) => o.value === value)
  const displayLabel = selectedOption?.label ?? label

  return (
    <div ref={containerRef} className={cn("relative", className)}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={cn(
          "inline-flex items-center gap-1 rounded-full px-3 py-1.5 text-xs font-medium border transition-colors",
          "min-h-11 md:min-h-8",
          value
            ? "bg-primary/10 text-primary border-primary/30"
            : "bg-muted/60 text-muted-foreground border-border"
        )}
      >
        {displayLabel}
        <ChevronDown className={cn("h-3 w-3 transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div
          className={cn(
            "absolute top-full left-0 mt-1 z-30",
            "rounded-lg border border-border/60 bg-popover shadow-lg",
            "min-w-[130px] py-1"
          )}
        >
          {options.map((opt) => (
            <button
              key={opt.value}
              type="button"
              onClick={() => {
                onChange(opt.value)
                setOpen(false)
              }}
              className={cn(
                "w-full px-3 py-2 text-sm text-left hover:bg-muted transition-colors cursor-pointer",
                opt.value === value ? "text-primary font-medium" : "text-popover-foreground"
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
