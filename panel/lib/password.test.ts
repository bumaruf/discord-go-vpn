import { describe, expect, it } from 'vitest'
import { hashPassword, verifyPassword } from './password'

// Made by @node-rs/argon2 with the same parameters, before the panel moved to Cloudflare: the
// ADMIN_PASSWORD_HASH values already deployed must keep working.
const NATIVE_HASH =
  '$argon2id$v=19$m=19456,t=2,p=1$IA3x+l627Pf+dEMjtnfocw$B2GTzMBzqaWB8V1EdrSWAKrHZCtpP0cyMOULTw6r+7k'

describe('verifyPassword', () => {
  it('accepts the password of a hash made by @node-rs/argon2', async () => {
    expect(await verifyPassword('senha-do-admin-123', NATIVE_HASH)).toBe(true)
  })

  it('rejects another password', async () => {
    expect(await verifyPassword('senha-errada-123', NATIVE_HASH)).toBe(false)
  })

  it.each(['', 'not a hash', '$argon2i$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA', '$argon2id$v=19$x'])(
    'treats %j as a mismatch',
    async (stored) => {
      expect(await verifyPassword('senha-do-admin-123', stored)).toBe(false)
    },
  )
})

describe('hashPassword', () => {
  it('makes a salted argon2id hash that verifies', async () => {
    const first = await hashPassword('senha-do-admin-123')
    const second = await hashPassword('senha-do-admin-123')

    expect(first).toMatch(/^\$argon2id\$v=19\$m=19456,t=2,p=1\$/)
    expect(first).not.toBe(second)
    expect(await verifyPassword('senha-do-admin-123', first)).toBe(true)
  })
})
