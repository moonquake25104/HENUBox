import { Button } from "@/components/ui/button"
import { Modal } from "@/components/shared/modal"
import { Save } from "lucide-react"
import { FIELD_CLASS, COLOR_OPTIONS, DAY_LABELS, type CourseFormData } from "./utils"

interface CourseFormModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isEditing: boolean
  form: CourseFormData
  setForm: (form: CourseFormData) => void
  saving: boolean
  onSave: () => void
}

export function CourseFormModal({
  open,
  onOpenChange,
  isEditing,
  form,
  setForm,
  saving,
  onSave,
}: CourseFormModalProps) {
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={isEditing ? "编辑课程" : "新建课程"}
      description="填写课程的时间、周次与地点等信息"
      className="max-w-2xl"
    >
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="col-span-2">
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            课程名称 <span className="text-destructive">*</span>
          </label>
          <input
            type="text"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            required
            placeholder="课程名称"
            className={FIELD_CLASS}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">星期</label>
          <select
            value={form.day_of_week}
            onChange={(e) => setForm({ ...form, day_of_week: Number(e.target.value) })}
            className={FIELD_CLASS}
          >
            {DAY_LABELS.map((label, i) => (
              <option key={i + 1} value={i + 1}>
                {label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">颜色</label>
          <select
            value={form.color}
            onChange={(e) => setForm({ ...form, color: e.target.value })}
            className={FIELD_CLASS}
          >
            {COLOR_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">开始时间</label>
          <input
            type="time"
            value={form.start_time}
            onChange={(e) => setForm({ ...form, start_time: e.target.value })}
            className={FIELD_CLASS}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">结束时间</label>
          <input
            type="time"
            value={form.end_time}
            onChange={(e) => setForm({ ...form, end_time: e.target.value })}
            className={FIELD_CLASS}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">周次</label>
          <input
            type="text"
            value={form.week_pattern}
            onChange={(e) => setForm({ ...form, week_pattern: e.target.value })}
            placeholder="如：1-17周全周"
            className={FIELD_CLASS}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            学期开始日期
          </label>
          <input
            type="date"
            value={form.semester_start_date}
            onChange={(e) => setForm({ ...form, semester_start_date: e.target.value })}
            className={FIELD_CLASS}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">地点</label>
          <input
            type="text"
            value={form.location}
            onChange={(e) => setForm({ ...form, location: e.target.value })}
            placeholder="教室/地点"
            className={FIELD_CLASS}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">教师</label>
          <input
            type="text"
            value={form.teacher}
            onChange={(e) => setForm({ ...form, teacher: e.target.value })}
            placeholder="授课教师"
            className={FIELD_CLASS}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">学期</label>
          <input
            type="text"
            value={form.semester}
            onChange={(e) => setForm({ ...form, semester: e.target.value })}
            placeholder="如：2026S1"
            className={FIELD_CLASS}
          />
        </div>
      </div>

      <div className="mt-5 flex items-center justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => onOpenChange(false)}
        >
          取消
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={saving || !form.name.trim()}
          onClick={onSave}
        >
          <Save className="mr-1 h-3.5 w-3.5" />
          {saving ? "保存中..." : "保存"}
        </Button>
      </div>
    </Modal>
  )
}
