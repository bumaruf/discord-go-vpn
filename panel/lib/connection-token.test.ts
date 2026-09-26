import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildConnectionToken, connectionTokenFor } from './connection-token'

const payload = {
  server: 'https://203.0.113.10:51821',
  fingerprint: 'q5H0x8vV2Zb1m3yKf7nWc4sPj6tRa9dLe2gUh8iYkQo=',
  invite: 'vpn_abcdefgh-ijkmnpqr-stuvwxyz',
}

function decode(token: string): unknown {
  return JSON.parse(Buffer.from(token.slice('dgv1_'.length), 'base64url').toString('utf8'))
}

afterEach(() => {
  vi.unstubAllEnvs()
})

describe('buildConnectionToken', () => {
  it('is dgv1_ plus unpadded base64url with no characters that break when pasted', () => {
    expect(buildConnectionToken(payload)).toMatch(/^dgv1_[A-Za-z0-9_-]+$/)
  })

  it('encodes the payload keys in the order server, fingerprint, invite', () => {
    const token = buildConnectionToken({
      invite: payload.invite,
      fingerprint: payload.fingerprint,
      server: payload.server,
    })

    expect(Object.keys(decode(token) as object)).toEqual(['server', 'fingerprint', 'invite'])
    expect(decode(token)).toEqual(payload)
  })
})

describe('connectionTokenFor', () => {
  it('uses the lease URL and fingerprint from the environment', () => {
    vi.stubEnv('PROVISIOND_LEASE_URL', payload.server)
    vi.stubEnv('PROVISIOND_FINGERPRINT', payload.fingerprint)

    expect(connectionTokenFor(payload.invite)).toBe(buildConnectionToken(payload))
  })

  it('drops a trailing slash from the lease URL', () => {
    vi.stubEnv('PROVISIOND_LEASE_URL', `${payload.server}/`)
    vi.stubEnv('PROVISIOND_FINGERPRINT', payload.fingerprint)

    expect(connectionTokenFor(payload.invite)).toBe(buildConnectionToken(payload))
  })

  it.each([
    ['PROVISIOND_LEASE_URL', 'PROVISIOND_FINGERPRINT'],
    ['PROVISIOND_FINGERPRINT', 'PROVISIOND_LEASE_URL'],
  ])('is undefined without %s', (missing, present) => {
    vi.stubEnv(missing, '')
    vi.stubEnv(present, 'value')

    expect(connectionTokenFor(payload.invite)).toBeUndefined()
  })
})
