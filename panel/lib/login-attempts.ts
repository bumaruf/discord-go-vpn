import 'server-only'

import { collection } from './mongodb'
import { COLLECTION, FIELD } from './mongo-names'

/**
 * Failed logins counted per address in Mongo, since serverless instances share no memory. Every
 * call fails open: login comes from env and must keep working while Atlas is down.
 */

export const MAX_FAILED_LOGINS = 10
export const LOCKOUT_MINUTES = 15

interface LoginAttemptDocument {
  /** The client address. */
  [FIELD.id]: string
  [FIELD.attempts]: number
  /** Also the TTL index, so a finished lockout leaves nothing behind. */
  [FIELD.expiresAt]: Date
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

function attemptsCollection() {
  return collection<LoginAttemptDocument>(COLLECTION.loginAttempts)
}

export async function loginBlocked(address: string): Promise<boolean> {
  try {
    const attempts = await attemptsCollection()
    const doc = await attempts.findOne({
      [FIELD.id]: address,
      [FIELD.expiresAt]: { $gt: new Date() },
    })
    return (doc?.[FIELD.attempts] ?? 0) >= MAX_FAILED_LOGINS
  } catch (error) {
    console.error('failed to read login attempts', error)
    return false
  }
}

/** The lockout window starts at the first failure and isn't extended by later ones. */
export async function recordFailedLogin(address: string): Promise<void> {
  try {
    const attempts = await attemptsCollection()
    const now = new Date()
    const counted = await attempts.updateOne(
      { [FIELD.id]: address, [FIELD.expiresAt]: { $gt: now } },
      { $inc: { [FIELD.attempts]: 1 } },
    )
    if (counted.matchedCount === 1) return

    // No window open, or one that ended but the TTL monitor hasn't removed yet.
    await attempts.updateOne(
      { [FIELD.id]: address },
      {
        $set: {
          [FIELD.attempts]: 1,
          [FIELD.expiresAt]: new Date(now.getTime() + LOCKOUT_MINUTES * 60_000),
        },
      },
      { upsert: true },
    )
  } catch (error) {
    console.error('failed to record a failed login', error)
  }
}

export async function clearFailedLogins(address: string): Promise<void> {
  try {
    const attempts = await attemptsCollection()
    await attempts.deleteOne({ [FIELD.id]: address })
  } catch (error) {
    console.error('failed to clear login attempts', error)
  }
}
