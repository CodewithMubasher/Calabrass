import { useSyncExternalStore } from "react"

/**
 * Who the app lets in, and who is signed in.
 *
 * One account, checked here in the browser because the API has no notion of a
 * user yet: this is the shape of the login screen, not a lock on anything — the
 * server answers anyone who can reach it, and these three lines are in the
 * bundle for anyone to read. All of it moves to the backend, and the app starts
 * checking on every request, when there is a session for the server to check.
 */
const ACCOUNT = {
  name: "Admin",
  email: "admin@cal.com",
  password: "Zorin1122",
}

/** Where the signed-in account is kept, so a reload does not log out. */
const STORAGE_KEY = "calabrass.session"

export interface Session {
  name: string
  email: string
}

export type SignInResult =
  | { ok: true; session: Session }
  /** Turned away: the same message either way, so the form says nothing about
   *  which half was wrong. */
  | { ok: false; error: string }

export function signIn(email: string, password: string): SignInResult {
  if (
    email.trim().toLowerCase() !== ACCOUNT.email ||
    password !== ACCOUNT.password
  ) {
    return { ok: false, error: "That email and password do not match." }
  }
  const next: Session = { name: ACCOUNT.name, email: ACCOUNT.email }
  store(next)
  return { ok: true, session: next }
}

export function signOut() {
  store(null)
}

/** The signed-in account, or null. */
export function session(): Session | null {
  return snapshot()
}

/**
 * The signed-in account as a component sees it: signing in or out redraws
 * whatever is showing it, which is what the sidebar's footer needs.
 */
export function useSession(): Session | null {
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}

// --- the session, in one place ----------------------------------------------

const listeners = new Set<() => void>()
let current: Session | null = null
let primed = false

/**
 * The session as it stands. The first read primes it from storage and every
 * later one answers from memory: useSyncExternalStore compares snapshots by
 * identity, so this must not build a new object on each call.
 */
function snapshot(): Session | null {
  if (!primed) {
    current = read()
    primed = true
  }
  return current
}

function store(next: Session | null) {
  current = next
  primed = true
  try {
    if (next) localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
    else localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Storage refused — private mode, or a full quota. The session then lasts
    // for this page only, which is still a session.
  }
  for (const listener of listeners) listener()
}

function read(): Session | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<Session> | null
    if (
      !parsed ||
      typeof parsed.name !== "string" ||
      typeof parsed.email !== "string"
    ) {
      return null
    }
    return { name: parsed.name, email: parsed.email }
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
