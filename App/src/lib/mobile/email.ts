import { registerPlugin } from "@capacitor/core"
interface EmailBrowser { open(): Promise<void> }
export const emailBrowser = registerPlugin<EmailBrowser>("XiqueEmail")
