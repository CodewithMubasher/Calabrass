import { useSyncExternalStore } from "react"

/**
 * Who is signed in.
 *
 * Accounts and passwords live on the backend: sign-up and login post to
 * `/api/auth/*`, which checks a bcrypt hash and hands back a bearer token. This
 * module only keeps that token (and the account it belongs to) for the browser,
 * so a reload stays signed in and the sidebar can show who it is.
 *
 * The token is attached to every API request, including the Server-Sent Events
 * and voice WebSocket streams, because the backend scopes chats, providers,
 * workspaces and usage to the signed-in account.
 */

/** Where the signed-in account is kept, so a reload does not log out. */
const STORAGE_KEY = "calabrass.auth"

/** The account a session belongs to, as the server describes it. */
export interface AuthUser {
  id: string
  name: string
  email: string
}

/** A signed-in account: the bearer token plus who it belongs to. */
export interface AuthSession {
  token: string
  user: AuthUser
}

export type Session = AuthUser

const listeners = new Set<() => void>()
let current: AuthSession | null = null
let primed = false

/**
 * The bearer token for the signed-in account, or null when nobody is signed in.
 * The API layer attaches it to every request.
 */
export function getToken(): string | null {
  return currentSession()?.token ?? null
}

/** The whole stored session, used by sign-in, sign-out and the refresh check. */
export function getSession(): AuthSession | null {
  return currentSession()
}

/** Replace the signed-in account. Null signs out. */
export function setSession(next: AuthSession) {
  current = next
  primed = true
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    // Storage refused — private mode, or a full quota. The session then lasts
    // for this page only, which is still a session.
  }
  for (const listener of listeners) listener()
}

/** Drop the signed-in account without waiting for the server to forget the token. */
export function clearSession() {
  current = null
  primed = true
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Same as above: a browser that refuses storage still has this page's view.
  }
  for (const listener of listeners) listener()
}

/** The signed-in account, or null. */
export function session(): Session | null {
  return currentSession()?.user ?? null
}

/**
 * The signed-in account as a component sees it: signing in or out redraws
 * whatever is showing it, which is what the sidebar's footer needs.
 */
export function useSession(): Session | null {
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}

// --- the session, in one place ----------------------------------------------

/**
 * The session as it stands. The first read primes it from storage and every
 * later one answers from memory: useSyncExternalStore compares snapshots by
 * identity, so this must not build a new object on each call.
 */
function snapshot(): Session | null {
  return currentSession()?.user ?? null
}

function currentSession(): AuthSession | null {
  if (!primed) {
    current = read()
    primed = true
  }
  return current
}

function read(): AuthSession | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as
      | { token?: unknown; user?: Partial<AuthUser> }
      | null
    if (
      !parsed ||
      typeof parsed.token !== "string" ||
      !parsed.user ||
      typeof parsed.user.id !== "string" ||
      typeof parsed.user.name !== "string" ||
      typeof parsed.user.email !== "string"
    ) {
      return null
    }
    return {
      token: parsed.token,
      user: {
        id: parsed.user.id,
        name: parsed.user.name,
        email: parsed.user.email,
      },
    }
  } catch {
    // Unreadable storage, or it held something that was never ours.
    return null
  }
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
