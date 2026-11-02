import { useId } from "react"

export type CampusKind = "electricity" | "classroom" | "library" | "email" | "focus" | "todos" | "timetable" | "guide"
/** Small code-native illustrations: no raster assets or background decoding cost. */
export function CampusThemeArt({ kind, glass }: { kind: CampusKind; glass: boolean }) {
  const id = useId().replace(/:/g, ""), fill = `url(#${id}-face)`, edge = `url(#${id}-edge)`
  const shapes = {
    classroom: <><path d="m143 231 51-35 55 35v69H143z" /><path d="m133 231 61-43 66 43M156 245h20v18h-20zm48 0h24v18h-24zm-48 31h20v20m30-20h22v20" /></>,
    electricity: <><rect x="167" y="183" width="69" height="107" rx="9" /><path d="M189 174h25m-13 26-19 39h21l-12 32 28-45h-21zM180 302h45" /></>,
    library: <><path d="M137 287v-65q28-18 59 2 30-20 61-2v65q-31-17-61 0-29-17-59 0zM196 224v63m-45-49q16-7 32 1m-32 12q16-7 32 1m26-13q16-7 33-1m-33 14q16-7 33-1" /></>,
    email: <><rect x="136" y="215" width="123" height="80" rx="9" /><path d="m138 221 59 44 60-44m-116 68 40-32m70 32-40-32" /></>,
    focus: <><circle cx="199" cy="250" r="50" /><path d="M199 217v35l24 15m-37-77h26m-13 0v10m-39 12-10-10m89 10 10-10" /></>,
    todos: <><rect x="157" y="199" width="86" height="104" rx="8" /><path d="M179 199v-9h43v9m-52 27 5 5 9-12m-14 30 5 5 9-12m-14 30 5 5 9-12m8-31h31m-31 23h31m-31 23h24" /></>,
    timetable: <><rect x="145" y="210" width="105" height="88" rx="10" /><path d="M145 234h105m-80-34v20m55-20v20m-60 31h9m17 0h9m17 0h9m-61 19h9m17 0h9m17 0h9" /></>,
    guide: <><path d="m135 235 40-14 41 14 40-14v74l-40 14-41-14-40 14zm40-14v74m41-60v74" /><path d="M210 233c-26-29-27-36-27-44a27 27 0 0 1 54 0c0 8-3 17-27 44z" /><circle cx="210" cy="190" r="8" /></>,
  }[kind]
  return <svg className={`xm-theme-art ${glass ? "is-glass" : "is-android"}`} viewBox="0 0 300 330" preserveAspectRatio="xMidYMax meet" fill="none" aria-hidden="true">
    {glass && <defs><linearGradient id={`${id}-face`} x1="130" y1="190" x2="260" y2="310" gradientUnits="userSpaceOnUse"><stop stopColor="#ffffff" stopOpacity=".8" /><stop offset=".52" stopColor="#ffffff" stopOpacity=".2" /><stop offset="1" stopColor="#a8bce6" stopOpacity=".6" /></linearGradient><linearGradient id={`${id}-edge`}><stop stopColor="#ffffff" stopOpacity=".95" /><stop offset="1" stopColor="#809fc7" stopOpacity=".5" /></linearGradient><radialGradient id={`${id}-orb`}><stop stopColor="#fff" stopOpacity=".75" /><stop offset="1" stopColor="#a9bce9" stopOpacity=".1" /></radialGradient></defs>}
    {glass ? <><ellipse cx="217" cy="256" rx="91" ry="81" fill={`url(#${id}-orb)`} /><path d="M0 291q74-43 153-13t147-12v64H0" fill="#ffffff" opacity=".2" /><path d="M0 306q80-35 156-13t144-6" stroke="#ffffff" opacity=".55" /><g transform="translate(11 7)" fill="#769bc0" opacity=".24" strokeWidth="3">{shapes}</g><g fill={fill} stroke={edge} strokeWidth="3" strokeLinejoin="round" strokeLinecap="round">{shapes}</g><path d="m243 176 3 9 9 3-9 3-3 9-3-9-9-3 9-3z" fill="#ffffff" opacity=".85" /></> : <><path d="M20 302h260M20 286h90m140-97v-20h30M20 265v-85h38" stroke="currentColor" opacity=".14" /><g stroke="currentColor" strokeWidth="3" strokeLinejoin="miter" strokeLinecap="square">{shapes}</g><path d="M23 311h27m4 0h5m4 0h5" stroke="currentColor" strokeWidth="2" opacity=".6" /></>}
  </svg>
}
