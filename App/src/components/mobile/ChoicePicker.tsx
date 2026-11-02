import { useRef, useState } from "react"
import { Check, ChevronDown } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import "./choice-picker.css"

export function ChoicePicker({ title, value, options, choose, weeks = false, disabled = false }: {
  title: string; value: string; options: { value: string; label: string }[];
  choose: (value: string) => void; weeks?: boolean; disabled?: boolean;
}) {
  const [open, setOpen] = useState(false), trigger = useRef<HTMLButtonElement>(null)
  const label = options.find(o => o.value === value)?.label ?? title
  return <>
    <button ref={trigger} type="button" className="xm-choice-trigger" aria-label={title} aria-haspopup="dialog" aria-expanded={open} disabled={disabled} onClick={() => setOpen(true)}><span>{label}</span><ChevronDown size={15} /></button>
    {open && <Modal open title={title} description={weeks ? "选择要查看的教学周。" : "为这段专注选择一个标签。"} layer={90} className="xm-choice-modal" returnFocusRef={trigger} onOpenChange={setOpen}>
      <div className={weeks ? "xm-choice-grid" : "xm-choice-list"} role="group" aria-label={`${title}选项`}>
        {options.map(o => <button key={o.value} type="button" aria-pressed={o.value === value} onClick={() => { choose(o.value); setOpen(false) }}><span>{o.label}</span>{!weeks && <Check size={17} aria-hidden="true" style={{ visibility: o.value === value ? "visible" : "hidden" }} />}</button>)}
      </div>
    </Modal>}
  </>
}
