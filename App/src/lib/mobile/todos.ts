import { Capacitor, registerPlugin } from "@capacitor/core"

export type TodoItem = { id: string; title: string; note: string; due_date: string; important: boolean; completed: boolean; created_at: number; updated_at: number; completed_at: number | null; revision: number }
export type TodoDraft = Pick<TodoItem, "title" | "note" | "due_date" | "important">
export type TodoState = { schema_version: 1; revision: number; today: string; items: TodoItem[]; counts: { open: number; completed: number; today: number; overdue: number } }
interface TodoBridge {
  getState(): Promise<TodoState>
  create(v: TodoDraft & { id: string }): Promise<TodoState>
  update(v: TodoDraft & { id: string; expected_revision: number }): Promise<TodoState>
  setCompleted(v: { id: string; expected_revision: number; completed: boolean }): Promise<TodoState>
  remove(v: { id: string; expected_revision: number }): Promise<TodoState>
}
export const todoBridge = registerPlugin<TodoBridge>("XiqueTodo")
export const todosAvailable = () => Capacitor.isPluginAvailable("XiqueTodo")
export function todoError(e: unknown) {
  const code = (e as { code?: string })?.code
  return ({ INVALID_ARGUMENT: "请检查内容：标题 1–120 字、备注最多 2000 字，计划日期须有效。", REVISION_CONFLICT: "事项已在其他位置更新。请关闭编辑后重新打开，核对最新内容再修改。", NOT_FOUND: "这条事项已被删除，列表已重新读取。", TODO_LIMIT: "最多保存 500 条事项，请先删除不再需要的已完成事项。" } as Record<string, string>)[code ?? ""] ?? "暂时无法读取或保存待办，请重试。"
}
export function todoDateLabel(item: TodoItem, today: string) {
  if (!item.due_date) return ""
  if (item.completed) return item.due_date.replaceAll("-", "/")
  return item.due_date < today ? "已逾期 · " + item.due_date.slice(5).replace("-", "/") : item.due_date === today ? "今天" : item.due_date.replaceAll("-", "/")
}
