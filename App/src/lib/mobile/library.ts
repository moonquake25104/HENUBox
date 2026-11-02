import { registerPlugin } from "@capacitor/core"

interface LibraryBrowser {
  open(): Promise<void>
}
// Native entry point fixes the official URL; remote pages never enter Capacitor.
export const libraryBrowser = registerPlugin<LibraryBrowser>("XiqueLibrary")
