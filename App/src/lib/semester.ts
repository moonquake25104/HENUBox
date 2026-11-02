export function formatSemester(semester: string): string {
  const match = /^\d{4}S([12])$/.exec(semester.trim())
  if (!match) return semester
  return match[1] === "1" ? "春季" : "秋季"
}
