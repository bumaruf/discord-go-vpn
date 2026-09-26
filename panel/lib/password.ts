import { hash, verify } from '@node-rs/argon2'

/** argon2id with the parameters OWASP recommends. */
const ARGON2_OPTIONS = { memoryCost: 19456, timeCost: 2, parallelism: 1 }

export function hashPassword(password: string): Promise<string> {
  return hash(password, ARGON2_OPTIONS)
}

export async function verifyPassword(password: string, storedHash: string): Promise<boolean> {
  try {
    return await verify(storedHash, password, ARGON2_OPTIONS)
  } catch {
    // A corrupt or non-argon2 hash is a mismatch, not an error that breaks the login route.
    return false
  }
}
