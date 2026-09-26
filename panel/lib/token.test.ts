import { createHash } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'

const randomBytes = vi.hoisted(() => ({ override: undefined as Buffer | undefined }))

vi.mock('node:crypto', async (importOriginal) => {
  const actual = await importOriginal<typeof import('node:crypto')>()
  return {
    ...actual,
    randomBytes: (size: number) => randomBytes.override ?? actual.randomBytes(size),
  }
})

import { generateInvite, hashToken } from './token'

const INVITE_FORMAT =
  /^vpn_[abcdefghijkmnpqrstuvwxyz23456789]{8}(-[abcdefghijkmnpqrstuvwxyz23456789]{8}){2}$/

afterEach(() => {
  randomBytes.override = undefined
})

describe('hashToken', () => {
  it('is the hex SHA-256 of the invite', () => {
    expect(hashToken('abc')).toBe(
      'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
    )
  })
})

describe('generateInvite', () => {
  it('generates vpn_ plus three groups of eight characters from the alphabet', () => {
    for (let i = 0; i < 50; i++) {
      expect(generateInvite().code).toMatch(INVITE_FORMAT)
    }
  })

  it('maps each random byte onto the alphabet modulo its length', () => {
    randomBytes.override = Buffer.from(Array.from({ length: 24 }, (_, i) => i + 32))

    expect(generateInvite().code).toBe('vpn_abcdefgh-ijkmnpqr-stuvwxyz')
  })

  it('returns the hash and the 12-character prefix of the generated invite', () => {
    const invite = generateInvite()

    expect(invite.hash).toBe(createHash('sha256').update(invite.code).digest('hex'))
    expect(invite.prefix).toBe(invite.code.slice(0, 12))
    expect(invite.prefix).toHaveLength(12)
  })

  it('does not repeat invites', () => {
    const invites = new Set(Array.from({ length: 100 }, () => generateInvite().code))
    expect(invites.size).toBe(100)
  })
})
