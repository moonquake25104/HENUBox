import { Modal } from "@/components/shared/modal"
import { ClipboardImportPanel, type ClipboardImportPanelProps } from "./import/ClipboardImportPanel"

interface ImportModalProps extends ClipboardImportPanelProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ImportModal({
  open,
  onOpenChange,
  ...clipboardImportProps
}: ImportModalProps) {
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="导入课表"
      description="从剪贴板导入课程"
      className="max-w-2xl"
    >
      <ClipboardImportPanel {...clipboardImportProps} />
    </Modal>
  )
}
