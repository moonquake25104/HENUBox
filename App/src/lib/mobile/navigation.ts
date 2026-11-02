import { CalendarDays, Grid2X2, House, UserRound } from "lucide-react"
export type MobilePage = "home" | "timetable" | "campus" | "me"
export const mobileNavigation = [
  { id: "home", title: "首页", icon: House }, { id: "timetable", title: "课表", icon: CalendarDays },
  { id: "campus", title: "校园", icon: Grid2X2 }, { id: "me", title: "我的", icon: UserRound },
] as const
