import { ApiError, api } from "@/lib/api"
import {
  clearSession,
  getSession,
  session,
  setSession,
  useSession,
  type Session,
} from "@/lib/session"
import { loadProviders } from "@/lib/provider-store"

export { session, useSession }
export type { Session }

export type SignInResult =
  | { ok: true; session: Session }
  | { ok: false; error: string }

/**
 * Accounts and passwords live on the backend now: sign-up and login post to
 * `/api/auth/*`, which checks a bcrypt hash and hands back a bearer token. This
 * module keeps that token (and the account it belongs to) for the browser, so a
 * reload stays signed in and the sidebar can show who it is.
 */

export async function signIn(
  email: string,
  password: string
): Promise<SignInResult> {
  try {
    const next = await api.signIn({ email: email.trim(), password })
    setSession(next)
    // Force reload providers after signin
    await loadProviders(true)
    return { ok: true, session: next.user }
  } catch (error) {
    return { ok: false, error: messageFrom(error) }
  }
}

export async function signUp(
  name: string,
  email: string,
  password: string
): Promise<SignInResult> {
  try {
    const next = await api.signUp({
      name: name.trim(),
      email: email.trim(),
      password,
    })
    setSession(next)
    // Force reload providers after signup
    await loadProviders(true)
    return { ok: true, session: next.user }
  } catch (error) {
    return { ok: false, error: messageFrom(error) }
  }
}

/**
 * Checks the stored token against the server. A token the server no longer
 * accepts signs the browser out; a server that cannot be reached leaves the
 * stored session alone, so a backend restart is not a logout.
 */
export async function refreshSession(): Promise<Session | null> {
  const stored = getSession()
  if (!stored) return null

  try {
    const { user } = await api.me()
    setSession({ token: stored.token, user })
    return user
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      clearSession()
      return null
    }
    return session()
  }
}

export function signOut() {
  const token = getSession()?.token
  clearSession()
  if (token) {
    void api.signOut(token).catch(() => {
      // The local session is already gone; a failed revoke only leaves an
      // expired row for the next cleanup.
    })
  }
}

function messageFrom(error: unknown): string {
  if (error instanceof ApiError) return error.message
  return "Could not reach the local server. Is it running?"
}