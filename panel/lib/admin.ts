import 'server-only'

import { timingSafeEqual } from 'node:crypto'
import { adminPassword, adminPasswordHash, adminUsername } from './env'
import { verifyPassword } from './password'

/**
 * The panel's single admin account comes from env, not from the database: VPN users have no
 * account and connect with an invite code. Login therefore keeps working while Atlas is down.
 */

export interface FixedAdmin {
  id: string
  username: string
  name: string
}

/** `null` on every failure, so a wrong username can't be told apart from a wrong password. */
export async function verifyAdmin(username: string, password: string): Promise<FixedAdmin | null> {
  const expected = adminUsername()
  const hash = adminPasswordHash()
  const plain = adminPassword()

  if (!expected || (!hash && !plain)) {
    // No default credentials: a factory admin/admin would survive into production.
    console.error('set ADMIN_USERNAME and ADMIN_PASSWORD_HASH (or ADMIN_PASSWORD)')
    return null
  }

  // Both comparisons always run: returning early would reveal through timing that the username
  // exists.
  const usernameMatches = constantTimeEqual(username, expected)
  const passwordMatches = hash
    ? await verifyPassword(password, decodeHash(hash))
    : constantTimeEqual(password, plain ?? '')

  if (!usernameMatches || !passwordMatches) return null

  return {
    id: 'admin',
    username: expected,
    name: expected,
  }
}

/** Accepts the hash as base64 or as a raw PHC string; why base64 is in panel/README.md. */
function decodeHash(value: string): string {
  if (value.startsWith('$argon2')) return value

  try {
    const raw = Buffer.from(value, 'base64').toString('utf8')
    return raw.startsWith('$argon2') ? raw : value
  } catch {
    return value
  }
}

/**
 * `===` leaks through timing how many leading characters matched. The length check comes first
 * because `timingSafeEqual` needs equal-length buffers, and it reveals only the length.
 */
function constantTimeEqual(received: string, expected: string): boolean {
  const a = Buffer.from(received, 'utf8')
  const b = Buffer.from(expected, 'utf8')
  if (a.length !== b.length) return false
  return timingSafeEqual(a, b)
}
