/**
 * Screens that carry the bar across the top: the home screen and a chat. Every
 * other page is its own full-height surface, the way the reference project lays
 * them out. The artifacts rail follows the same line — it is opened from that
 * bar's file icon.
 */
export function hasTopbar(pathname: string) {
  return pathname === "/" || pathname.startsWith("/chat")
}
