import { createHash, randomBytes } from 'node:crypto'

// The invite code is the credential inside the connection token (lib/connection-token.ts); the
// client never checks its shape, only provisiond checks its hash.
const ALPHABET = 'abcdefghijkmnpqrstuvwxyz23456789'
const GROUPS = 3
const CHARS_PER_GROUP = 8

export interface GeneratedInvite {
  /** Plaintext, shown only once to the admin who generated it. */
  code: string
  hash: string
  /** Lets the admin tell invites apart. */
  prefix: string
}

export function generateInvite(): GeneratedInvite {
  // 24 characters from 32 symbols: 120 bits.
  const bytes = randomBytes(GROUPS * CHARS_PER_GROUP)
  const letters = Array.from(bytes, (b) => ALPHABET[b % ALPHABET.length])

  const groups: string[] = []
  for (let i = 0; i < GROUPS; i++) {
    groups.push(letters.slice(i * CHARS_PER_GROUP, (i + 1) * CHARS_PER_GROUP).join(''))
  }

  const code = `vpn_${groups.join('-')}`
  return { code, hash: hashToken(code), prefix: code.slice(0, 12) }
}

/**
 * Plain SHA-256, not argon2: a slow hash only helps against guessable secrets, and provisiond
 * checks this one on every lease. Must match internal/provisiond/auth/auth.go.
 */
export function hashToken(code: string): string {
  return createHash('sha256').update(code).digest('hex')
}
