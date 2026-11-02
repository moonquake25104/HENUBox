import { ChevronDown, RefreshCw, BookOpen, Plus, AlertCircle } from "lucide-react"
import { semesterLabel } from "./dates"
export function SemesterPicker({
  value,
  values,
  onChange,
}: {
  value: string
  values: string[]
  onChange: (value: string) => void
}) {
  return (
    <div className="xq-semester-picker">
      <BookOpen size={16} />
      <select
        aria-label="选择学期"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {values.map((item) => (
          <option key={item} value={item}>
            {semesterLabel(item)}
          </option>
        ))}
      </select>
      <ChevronDown size={14} />
    </div>
  )
}
export function EmptyCourses({ add }: { add: () => void }) {
  return (
    <div className="xq-empty">
      <span className="xq-empty-icon">
        <BookOpen size={27} strokeWidth={1.5} />
      </span>
      <h3>从第一门课程开始</h3>
      <p>添加上课时间与地点，让每一天的安排清晰起来。</p>
      <button className="xq-button primary" onClick={add}>
        <Plus size={16} />
        添加课程
      </button>
    </div>
  )
}
export function LoadError({ error, retry }: { error: string; retry: () => void }) {
  return (
    <div className="xq-error" role="alert">
      <AlertCircle size={20} />
      <div>
        <strong>暂时无法读取课程</strong>
        <p>{error}</p>
      </div>
      <button className="xq-button small" onClick={retry}>
        <RefreshCw size={14} />
        重试
      </button>
    </div>
  )
}
export function LoadingCards() {
  return (
    <div className="xq-loading" role="status" aria-label="正在加载课表">
      <div />
      <div />
      <div />
    </div>
  )
}
