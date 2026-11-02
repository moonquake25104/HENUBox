import type { Course } from "@/types/course"
import { Button } from "@/components/ui/button"
import { Modal } from "@/components/shared/modal"
import { Pencil, Trash2 } from "lucide-react"
import { DAY_LABELS } from "./utils"
import { formatSemester } from "@/lib/semester"

interface CourseDetailModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  course: Course | null
  onEdit: (course: Course) => void
  onDelete: (courseId: number) => void
}

export function CourseDetailModal({
  open,
  onOpenChange,
  course,
  onEdit,
  onDelete,
}: CourseDetailModalProps) {
  if (!course) return null

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="课程详情"
      className="max-w-sm"
    >
      <div className="space-y-4">
        <div className="flex items-center gap-3">
          <div
            className="h-4 w-4 shrink-0 rounded"
            style={{ backgroundColor: course.color }}
          />
          <h3 className="text-lg font-semibold text-foreground">{course.name}</h3>
        </div>
        <div className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <div className="text-xs text-muted-foreground">时间</div>
            <div className="mt-0.5 font-medium tabular-nums">
              {DAY_LABELS[course.day_of_week - 1]} {course.start_time}-{course.end_time}
            </div>
          </div>
          {course.location && (
            <div>
              <div className="text-xs text-muted-foreground">地点</div>
              <div className="mt-0.5 font-medium">{course.location}</div>
            </div>
          )}
          {course.teacher && (
            <div>
              <div className="text-xs text-muted-foreground">教师</div>
              <div className="mt-0.5 font-medium">{course.teacher}</div>
            </div>
          )}
          <div>
            <div className="text-xs text-muted-foreground">学期</div>
            <div className="mt-0.5 font-medium">{formatSemester(course.semester) || "—"}</div>
          </div>
          <div className="col-span-2">
            <div className="text-xs text-muted-foreground">周次</div>
            <div className="mt-0.5 font-medium">{course.week_pattern || "每 周"}</div>
          </div>
        </div>
        <div className="flex items-center gap-2 pt-2 border-t border-border/50">
          <Button
            size="sm"
            onClick={() => onEdit(course)}
          >
            <Pencil className="mr-1.5 h-3.5 w-3.5" />
            编辑
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:bg-destructive/10"
            onClick={() => onDelete(course.id)}
          >
            <Trash2 className="mr-1.5 h-3.5 w-3.5" />
            删除
          </Button>
        </div>
      </div>
    </Modal>
  )
}
