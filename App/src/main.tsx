import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import "./index.css"
import "./xique.css"
import App from "./App.tsx"
import { applyTheme } from "./hooks/use-theme"

// Apply theme before React hydrates — prevents flash
const initial = localStorage.getItem("kairos-theme")
if (initial === "dark" || initial === "light") {
  applyTheme(initial)
} else if (window.matchMedia("(prefers-color-scheme: dark)").matches) {
  applyTheme("dark")
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
