import { randomBytes, timingSafeEqual } from 'node:crypto'
import { argon2id } from '@noble/hashes/argon2.js'

// Pure JS rather than a native binding: the panel runs on a Cloudflare Worker, which loads no
// `.node` addons. The PHC strings are the same ones @node-rs/argon2 produced.

/** argon2id with the parameters OWASP recommends. */
const ARGON2_OPTIONS = { m: 19456, t: 2, p: 1 }
const SALT_BYTES = 16
const HASH_BYTES = 32
const VERSION = 0x13

const PHC = /^\$argon2id\$v=19\$m=(\d+),t=(\d+),p=(\d+)\$([A-Za-z0-9+/]+)\$([A-Za-z0-9+/]+)$/

function b64(bytes: Uint8Array): string {
  return Buffer.from(bytes).toString('base64').replace(/=+$/, '')
}

export async function hashPassword(password: string): Promise<string> {
  const { m, t, p } = ARGON2_OPTIONS
  const salt = randomBytes(SALT_BYTES)
  const hash = argon2id(password, salt, { m, t, p, dkLen: HASH_BYTES, version: VERSION })
  return `$argon2id$v=19$m=${m},t=${t},p=${p}$${b64(salt)}$${b64(hash)}`
}

export async function verifyPassword(password: string, storedHash: string): Promise<boolean> {
  // A corrupt or non-argon2id hash is a mismatch, not an error that breaks the login route.
  const match = PHC.exec(storedHash)
  if (!match) return false

  const [, m, t, p, salt, expected] = match
  const expectedBytes = Buffer.from(expected, 'base64')
  try {
    const actual = argon2id(password, Buffer.from(salt, 'base64'), {
      m: Number(m),
      t: Number(t),
      p: Number(p),
      dkLen: expectedBytes.length,
      version: VERSION,
    })
    return timingSafeEqual(actual, expectedBytes)
  } catch {
    return false
  }
}
