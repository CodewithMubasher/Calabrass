import { toast } from "@/components/ui/toast"

/**
 * Notices: the short messages that say what went wrong, or what just
 * happened.
 *
 * They are toasts rather than messages on the page, so a failure says itself
 * wherever the reader is instead of waiting to be found. A notice that has a
 * key keeps its place: the next one with the same key replaces it rather than
 * stacking on top of it, which is what a list that keeps failing to load
 * needs.
 */

/** How long a notice stays on the screen. */
const NOTICE_TIMEOUT = 5000

/**
 * What each keyed notice said last time. A list that keeps failing to load
 * says the same thing over and over, and a notice that says it again is only
 * noise: the same words under the same key are said once, until something
 * clears the key.
 */
const lastNotice = new Map<string, string>()

/** Forget a keyed notice, so the same thing said again is heard again. */
export function clearNotice(key: string) {
  lastNotice.delete(key)
}

/** Whether this notice is worth saying, and remember it either way. */
function worthSaying(key: string | undefined, description: string) {
  if (!key) return true
  if (lastNotice.get(key) === description) return false
  lastNotice.set(key, description)
  return true
}

export function notifyError(
  description: string,
  title = "Something went wrong",
  key?: string
) {
  if (!worthSaying(key, description)) return
  toast.add({
    ...(key ? { id: key } : {}),
    type: "error",
    title,
    description,
    timeout: NOTICE_TIMEOUT,
  })
}

export function notifySuccess(
  description: string,
  title = "Done",
  key?: string
) {
  if (key) clearNotice(key)
  toast.add({
    ...(key ? { id: key } : {}),
    type: "success",
    title,
    description,
    timeout: NOTICE_TIMEOUT,
  })
}

export function notifyInfo(
  description: string,
  title = "Heads up",
  key?: string
) {
  if (!worthSaying(key, description)) return
  toast.add({
    ...(key ? { id: key } : {}),
    type: "info",
    title,
    description,
    timeout: NOTICE_TIMEOUT,
  })
}

export function notifyWarning(
  description: string,
  title = "Careful",
  key?: string
) {
  if (!worthSaying(key, description)) return
  toast.add({
    ...(key ? { id: key } : {}),
    type: "warning",
    title,
    description,
    timeout: NOTICE_TIMEOUT,
  })
}

/** The message an unknown failure carries, or the fallback when it has none. */
export function reasonFor(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback
}
