/**
 * Screens that carry the bar across the top: the home screen and a chat. Every
 * other page is its own full-height surface, the way the reference project lays
 * them out. The artifacts rail follows the same line — it is opened from that
 * bar's file icon.
 */
export function hasTopbar(pathname: string) {
  return pathname === "/" || pathname.startsWith("/chat")
}

/**
 * Screens that stand on their own: no sidebar, no bar, no rail, no surface of
 * their own to sit in. The login page is the only one — it is reached before
 * there is an app to sit inside.
 */
export function isStandalone(pathname: string) {
  return pathname === "/login"
}

/**
 * The chat a path belongs to, or "". The rail is mounted above the routes, so
 * it reads the chat from the URL rather than from route params. No chat means
 * the workspace the project works in by default.
 */
export function chatIdFrom(pathname: string) {
  const match = /^\/chat\/([^/]+)/.exec(pathname)
  return match ? decodeURIComponent(match[1]) : ""
}
