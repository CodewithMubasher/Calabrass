import { useEffect } from "react"
import { useLocation } from "@tanstack/react-router"

import { useSidebar } from "@/components/ui/sidebar"
import { toggleArtifacts, toggleCrew } from "@/lib/panel-store"
import { hasTopbar } from "@/lib/routes"

/** True when the key event came from somewhere the user is typing. */
function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false
  }

  if (target.isContentEditable) {
    return true
  }

  return target.closest("input, textarea, select, [contenteditable='true']") !== null
}

/**
 * The bare-key shortcuts: `s` folds the sidebar, `d` opens the artifacts rail,
 * `c` opens the crew rail. Modifier combinations belong to the browser and the
 * sidebar's own ctrl+b, so anything with a modifier is left alone, and so is
 * anything typed into the composer — `s`, `d` and `c` are ordinary letters in a
 * sentence.
 *
 * Rendered inside `SidebarProvider`, which is where the sidebar's toggle comes
 * from.
 */
export function KeyboardShortcuts() {
  const { toggleSidebar } = useSidebar()
  const { pathname } = useLocation()
  const railAvailable = hasTopbar(pathname)

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.repeat) return
      if (event.metaKey || event.ctrlKey || event.altKey) return
      if (isEditableTarget(event.target)) return

      switch (event.key.toLowerCase()) {
        case "s":
          toggleSidebar()
          break
        case "d":
          if (railAvailable) toggleArtifacts()
          break
        case "c":
          if (railAvailable) toggleCrew()
          break
      }
    }

    window.addEventListener("keydown", handleKeyDown)

    return () => {
      window.removeEventListener("keydown", handleKeyDown)
    }
  }, [railAvailable, toggleSidebar])

  return null
}
