import 'server-only'

import { getCloudflareContext } from '@opennextjs/cloudflare'

/**
 * Failed logins counted per address in Workers KV, since Worker isolates share no memory. Every
 * call fails open: login comes from env and must keep working while KV is unavailable.
 */

export const MAX_FAILED_LOGINS = 10
export const LOCKOUT_MINUTES = 15

/** The value stored under `login:<address>`, expiring with the window. */
interface LoginAttempts {
  attempts: number
  /** Epoch seconds. */
  expiresAt: number
}

/**
 * `cf-connecting-ip`, which Cloudflare overwrites on every request. A client can put anything at
 * the start of `x-forwarded-for` there, so that header is only read off Cloudflare, by `next dev`.
 */
export function clientAddress(request: Request): string {
  const cloudflare = request.headers.get('cf-connecting-ip')
  if (cloudflare) return cloudflare

  const forwarded = request.headers.get('x-forwarded-for')?.split(',')[0]?.trim()
  return forwarded || request.headers.get('x-real-ip') || 'unknown'
}

function namespace(): KVNamespace {
  return getCloudflareContext().env.LOGIN_ATTEMPTS
}

function key(address: string): string {
  return `login:${address}`
}

async function read(address: string): Promise<LoginAttempts | null> {
  const stored = await namespace().get<LoginAttempts>(key(address), 'json')
  // KV deletes an expired key within a minute or so; until then it still reads.
  return stored && stored.expiresAt > Date.now() / 1000 ? stored : null
}

export async function loginBlocked(address: string): Promise<boolean> {
  try {
    return ((await read(address))?.attempts ?? 0) >= MAX_FAILED_LOGINS
  } catch (error) {
    console.error('failed to read login attempts', error)
    return false
  }
}

/** The lockout window starts at the first failure and isn't extended by later ones. */
export async function recordFailedLogin(address: string): Promise<void> {
  try {
    const current = await read(address)
    const next: LoginAttempts = current
      ? { attempts: current.attempts + 1, expiresAt: current.expiresAt }
      : { attempts: 1, expiresAt: Math.ceil(Date.now() / 1000) + LOCKOUT_MINUTES * 60 }
    // KV refuses an expiration less than 60 s away; read() still ends the window at expiresAt.
    const expiration = Math.max(next.expiresAt, Math.ceil(Date.now() / 1000) + 60)
    await namespace().put(key(address), JSON.stringify(next), { expiration })
  } catch (error) {
    console.error('failed to record a failed login', error)
  }
}

export async function clearFailedLogins(address: string): Promise<void> {
  try {
    await namespace().delete(key(address))
  } catch (error) {
    console.error('failed to clear login attempts', error)
  }
}
