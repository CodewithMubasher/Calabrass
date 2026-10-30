import { api } from "@/lib/api"
import { navigate } from "@/lib/navigation"
import { notifyError } from "@/lib/toast"

/**
 * What only the desktop app does for the page. Inside the desktop window the
 * Wails runtime is on `window`; in a browser or the terminal it is not, and
 * this does nothing.
 *
 * The desktop shell tells the page when a folder is dropped on the window. The
 * page, not the shell, makes the chat, so the chat belongs to whoever is signed
 * in here.
 */
interface WailsRuntime {
  EventsOn?: (event: string, callback: (...data: unknown[]) => void) => () => void
}

const FOLDER_DROPPED = "calabrass:folder-dropped"

export function connectDesktop(): () => void {
  const runtime = (window as unknown as { runtime?: WailsRuntime }).runtime
  if (!runtime?.EventsOn) return () => {}

  // The shell only treats an element as a drop target if it carries this
  // property; it inherits, so the root is enough.
  document.documentElement.style.setProperty("--wails-drop-target", "drop")

  const stop = runtime.EventsOn(FOLDER_DROPPED, (path) => {
    if (typeof path !== "string" || !path) return
    void api
      .createChatInFolder(path)
      // A literal path: this helper is not tied to the router's typed params, and
      // "/chat/<id>" is the same route as "/chat/$chatId".
      .then((chat) =>
        navigate({ to: `/chat/${encodeURIComponent(chat.id)}` } as unknown as Parameters<typeof navigate>[0])
      )
      .catch((err: unknown) =>
        notifyError(
          err instanceof Error ? err.message : "That folder could not be opened.",
          "Could not open the folder",
          "folder-drop"
        )
      )
  })

  return () => {
    stop?.()
    document.documentElement.style.removeProperty("--wails-drop-target")
  }
}
