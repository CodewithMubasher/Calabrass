/**
 * Screens that carry the bar across the top: the home screen, a chat and a crew
 * run. Every other page is its own full-height surface, the way the reference
 * project lays them out. The artifacts rail follows the same line — it is
 * opened from that bar's file icon.
 */
export function hasTopbar(pathname: string) {
  return (
    pathname === "/" ||
    pathname.startsWith("/chat") ||
    pathname.startsWith("/crew")
  )
}

/**
 * Screens that stand on their own: no sidebar, no bar, no rail, no surface of
 * their own to sit in. The login and sign-up pages are the two — they are
 * reached before there is an app to sit inside.
 */
export function isStandalone(pathname: string) {
  return pathname === "/login" || pathname === "/signup"
}

/**
 * The chat a path belongs to, or "". The rail is mounted above the routes, so
 * it reads the chat from the URL rather than from route params. No chat means
 * the workspace the project works in by default.
 *
 * Both addresses are read: a conversation is a chat whether it is under /chat
 * or under /crew, and the two differ only in the mode they run in.
 */
export function chatIdFrom(pathname: string) {
  const match = /^\/(?:chat|crew)\/([^/]+)/.exec(pathname)
  return match ? decodeURIComponent(match[1]) : ""
}

/**
 * Whether a path is a crew run. A crew conversation lives under /crew, and
 * that is what says which mode it is in.
 */
export function isCrewPath(pathname: string) {
  return pathname.startsWith("/crew")
}
