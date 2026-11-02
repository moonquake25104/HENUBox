import { useId, useLayoutEffect, type ReactNode, type RefObject } from "react"
import { setModalPresence } from "@/lib/mobile/modal-presence"
import { Dialog } from "radix-ui"
import { X } from "lucide-react"
import { cn } from "@/lib/utils"
import { useMobileBack } from "@/lib/mobile/back"

interface ModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  /** 无障碍描述；缺省时以标题作为视觉隐藏描述，避免 Radix 警告。 */
  description?: string
  children: ReactNode
  /** 覆盖内容区宽度等样式，例如 max-w-2xl。 */
  className?: string
  /** 弹窗变体：center（默认居中）| bottom（底部半屏滑出） */
  variant?: "center" | "bottom"
  /** Explicit layer for a dialog opened from another dialog. */
  layer?: number
  /** Restore focus for dialogs opened without a Radix Trigger. */
  returnFocusRef?: RefObject<HTMLElement | null>
}

/** 模态弹窗：支持居中（center）和底部半屏（bottom）两种变体，基于 Radix Dialog（含焦点陷阱与 Esc 关闭）。 */
export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  className,
  variant = "center",
  layer,
  returnFocusRef,
}: ModalProps) {
  const isBottom = variant === "bottom"
  const presenceId = useId()
  useLayoutEffect(() => { setModalPresence(presenceId, open); return () => setModalPresence(presenceId, false) }, [presenceId, open])
  useMobileBack(open, () => onOpenChange(false), 100)

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay
          style={layer === undefined ? undefined : { zIndex: layer }}
          className={cn(
            "fixed inset-0 z-50 bg-black/40 backdrop-blur-sm",
            "data-[state=open]:animate-in data-[state=closed]:animate-out",
            "data-[state=open]:fade-in-0 data-[state=closed]:fade-out-0",
          )}
        />
        <Dialog.Content
          onCloseAutoFocus={event => {
            if (returnFocusRef?.current?.isConnected) {
              event.preventDefault()
              returnFocusRef.current.focus({ preventScroll: true })
            }
          }}
          style={layer === undefined ? undefined : { zIndex: layer + 1 }}
          className={cn(
            isBottom
              ? cn(
                  "fixed inset-x-0 bottom-0 z-50 mx-auto w-full max-w-lg",
                  "rounded-t-2xl rounded-b-none",
                  "border border-b-0 border-border/60 bg-card/95 p-5 shadow-2xl",
                  "backdrop-blur-xl backdrop-saturate-150 focus:outline-none",
                  "max-h-[75vh] overflow-y-auto",
                  "data-[state=open]:animate-in data-[state=closed]:animate-out",
                  "data-[state=open]:fade-in-0 data-[state=closed]:fade-out-0",
                  "data-[state=open]:slide-in-from-bottom-full data-[state=closed]:slide-out-to-bottom-full",
                  "duration-300",
                )
              : cn(
                  "fixed left-1/2 top-1/2 z-50 w-[calc(100%-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2",
                  "rounded-2xl border border-border/60 bg-card/95 p-5 shadow-2xl",
                  "backdrop-blur-xl backdrop-saturate-150 focus:outline-none",
                  "data-[state=open]:animate-in data-[state=closed]:animate-out",
                  "data-[state=open]:fade-in-0 data-[state=closed]:fade-out-0",
                  "data-[state=open]:zoom-in-95 data-[state=closed]:zoom-out-95",
                  "data-[state=open]:slide-in-from-bottom-2",
                ),
            className,
          )}
        >
          <div className="mb-4 flex items-start justify-between gap-4">
            <div className="min-w-0">
              <Dialog.Title className="font-heading text-base font-medium text-foreground">
                {title}
              </Dialog.Title>
              {description ? (
                <Dialog.Description className="mt-0.5 text-xs text-muted-foreground">
                  {description}
                </Dialog.Description>
              ) : (
                <Dialog.Description className="sr-only">{title}</Dialog.Description>
              )}
            </div>
            <Dialog.Close
              className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="关闭"
            >
              <X className="h-4 w-4" />
            </Dialog.Close>
          </div>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
