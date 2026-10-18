import type { IconSvgElement } from "@hugeicons/react"
import {
  FileSpreadsheetIcon,
  ServerStack01Icon,
  WindowsNewIcon,
} from "@hugeicons/core-free-icons"

/**
 * The glyph an MCP server goes by. A connector is named by whoever added it, so
 * that name is the only handle the UI has on it: the servers we ship get their
 * own icon, and anything else gets a rack to say "a server did this".
 *
 * One resolver for the whole app, so a connector wears the same mark on its card
 * as it does on the timeline node of the calls it answers.
 */
const connectorIcons: Record<string, IconSvgElement> = {
  excel: FileSpreadsheetIcon,
  "win-control": WindowsNewIcon,
}

const connectorIconRules: Array<[RegExp, IconSvgElement]> = [
  [/excel|spreadsheet|xlsx/i, FileSpreadsheetIcon],
  [/win[\s-]*control|windows/i, WindowsNewIcon],
]

export function connectorIcon(name: string): IconSvgElement {
  const key = name.trim().toLowerCase()
  return (
    connectorIcons[key] ??
    connectorIconRules.find(([pattern]) => pattern.test(key))?.[1] ??
    ServerStack01Icon
  )
}
