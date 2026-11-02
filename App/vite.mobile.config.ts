// Codex-owned mobile frontend build; shell build scripts should select this config.
import { mergeConfig } from "vite"
import webConfig from "./vite.config"
import path from "node:path"

export default mergeConfig(webConfig, {
  plugins: [{
    name: "xique-mobile-document-boundary",
    transformIndexHtml: () => [{
      tag: "meta",
      attrs: { "http-equiv": "Content-Security-Policy", content: "frame-src 'none'; object-src 'none'; base-uri 'self'" },
      injectTo: "head-prepend" as const,
    }],
  }],
  define: { "import.meta.env.VITE_XIQUE_PLATFORM": JSON.stringify("android") },
  build: { outDir: path.resolve(__dirname, "../runtime/build/mobile-web"), emptyOutDir: true },
})
