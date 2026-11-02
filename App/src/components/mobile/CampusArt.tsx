import { useUiStyle } from "@/lib/mobile/ui-style"
import { CampusThemeArt } from "./CampusThemeArt"
/** Original vector illustrations, kept in code so cards scale cleanly. */
export function CampusArt({ kind }: { kind: "electricity" | "classroom" | "library" | "email" | "focus" | "todos" | "guide" }) {
  const { style } = useUiStyle()
  if (style !== "classic") return <CampusThemeArt kind={kind} glass={style === "glass"} />
  if (kind === "guide") return <svg viewBox="0 0 300 330" preserveAspectRatio="xMidYMax slice" fill="none" aria-hidden="true"><path d="M0 288q75-30 138-7t162-9v58H0z" fill="#9cbba3" /><path d="m117 219 47-17 44 17 47-17v102l-47 17-44-17-47 17z" fill="#f3f4dc" /><path d="M164 202v102m44-85v102" stroke="#b2c8ae" strokeWidth="3" /><path d="m126 286 24-31 31 16 32-26 30 10" stroke="#93b49d" strokeWidth="4" strokeDasharray="6 7" /><path d="M203 211c-37-40-34-52-34-58a34 34 0 0 1 68 0c0 8 0 18-34 58z" fill="#648b78" /><circle cx="203" cy="154" r="12" fill="#e9f4e5" /></svg>
  if (kind === "todos") return <svg viewBox="0 0 300 330" preserveAspectRatio="xMidYMax slice" fill="none" aria-hidden="true">
    <path d="M0 290q70-28 137-7t163-5v52H0z" fill="#d3b573" />
    <g transform="rotate(9 207 238)"><rect x="136" y="156" width="129" height="151" rx="15" fill="#fff9e8" /><rect x="174" y="145" width="53" height="21" rx="8" fill="#b49b5f" />
    <path d="m153 196 7 7 13-15m-20 41 7 7 13-15m-20 40 7 7 13-15" stroke="#799781" strokeWidth="5" strokeLinecap="round" strokeLinejoin="round" /><path d="M186 197h55m-55 33h55m-55 33h40" stroke="#d8c797" strokeWidth="5" strokeLinecap="round" /></g>
  </svg>
  if (kind === "focus") return <svg viewBox="0 0 300 330" preserveAspectRatio="xMidYMax slice" fill="none" aria-hidden="true">
    <path d="M0 290q68-35 122-8t178-6v54H0" fill="#dcad88" />
    <ellipse cx="203" cy="283" rx="65" ry="10" fill="#c6916c" opacity=".4" />
    <rect x="169" y="145" width="65" height="8" rx="4" fill="#9c6f50" />
    <circle cx="201" cy="219" r="56" fill="#fff8e9" /><circle cx="201" cy="219" r="47" stroke="#c89571" strokeWidth="4" />
    <path d="M201 184v37l22 14" stroke="#855c42" strokeWidth="6" strokeLinecap="round" />
    <path d="M109 263v-42m0 22q-23-5-23-24 24 0 23 24m1-10q21-3 22-24-22 1-22 24" stroke="#73957f" strokeWidth="7" strokeLinecap="round" />
    <path d="M89 260h42l-6 25H95z" fill="#f8f0df" />
  </svg>
  if (kind === "electricity") return <svg viewBox="0 0 300 330" preserveAspectRatio="xMidYMax slice" fill="none" aria-hidden="true">
    <path d="M0 288q50-42 101-13t95-8 104 21v42H0z" fill="#47ad94"/>
    <path d="M0 160q18-16 31 0 20-35 41-4 21-5 34 12H0m168 46q19-24 39-6 27-38 54-5 23-9 39 8H168" fill="#fff"/>
    <path d="M170 91h16v239h-16z" fill="#35496a"/><path d="M175 26h6v85h-6z" fill="#fff"/><path d="M175 39h6v12h-6zm0 26h6v12h-6z" fill="#db9b9c"/>
    <ellipse cx="178" cy="163" rx="30" ry="34" fill="#504e8c"/><path d="M178 129c-15 22-15 46 0 68 14-23 14-47 0-68" fill="#7475ab"/>
    <path d="M71 250l23-36-5 28h17l-25 34 5-26z" fill="#fff3b9"/>
  </svg>
  if (kind === "classroom") return <svg viewBox="0 0 300 380" preserveAspectRatio="xMidYMax slice" fill="none" aria-hidden="true">
    <path d="M0 220q50-43 98-9t88-2 114 3v168H0z" fill="#fff"/>
    <path d="M0 274l31-13 32 29 44-39 45 21 47-55 42 56 26-14 33 24v97H0z" fill="#b9e9ed"/>
    <path d="M0 329l49-32 47 26 43-19 62 45 55-33 44 21v43H0z" fill="#d5f3f2"/>
    <path d="M150 285h74v70h-74z" fill="#8cbec5"/><path d="M145 285l42-34 43 34z" fill="#fff"/>
    <path d="M163 299h12v14h-12zm34 0h12v14h-12zm-34 25h12v14h-12zm34 0h12v14h-12z" fill="#efffff"/>
  </svg>
  if (kind === "email") return <svg viewBox="0 0 300 330" preserveAspectRatio="xMidYMax slice" fill="none" aria-hidden="true">
    <path d="M0 290q70-45 143-10t157-9v59H0z" fill="#96B6DF" />
    <path d="M0 184q18-21 38-8 18-34 45-6 22-6 40 12H0m190 32q18-21 36-7 20-24 41-5 17-8 33 8v11H190z" fill="#F4F8FF" />
    <g transform="rotate(-9 175 249)"><rect x="79" y="185" width="183" height="116" rx="14" fill="#F5F8FF" />
      <path d="M81 194l89 66 90-65" stroke="#759BCD" strokeWidth="9" strokeLinejoin="round" />
      <path d="M82 293l60-50m115 50l-57-50" stroke="#C5D8F0" strokeWidth="7" strokeLinecap="round" />
    </g><path d="M182 141l27 8-27 12 4-10-4-10z" fill="#7297C6" /><path d="M135 169q-16-30 26-31" stroke="#98B3D7" strokeWidth="3" strokeDasharray="5 7" strokeLinecap="round" />
  </svg>
  return <svg viewBox="0 0 300 330" preserveAspectRatio="xMidYMax slice" fill="none" aria-hidden="true">
    <path d="M0 137q19-20 39-5 25-40 53-6 28-8 46 12H0m169-49q19-22 40-8 26-32 50-3 24-9 41 7v12H169z" fill="#fff"/>
    <path d="M0 304h300v26H0z" fill="#86cdd5"/>
    <path d="M37 193h18v137H37zm208 0h18v137h-18z" fill="#ef9db7"/>
    <path d="M40 184h12v25H40zm208 0h12v25h-12z" fill="#f7bed0"/>
    <path d="M46 218q104 133 208 0" stroke="#f6b6cb" strokeWidth="12"/>
    <path d="M46 294h208" stroke="#f6b6cb" strokeWidth="12"/>
    <path d="M78 249v46m35-19v19m37-8v8m37-19v19m35-46v46" stroke="#f6b6cb" strokeWidth="6"/>
  </svg>
}
