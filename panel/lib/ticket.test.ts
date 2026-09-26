import { createHmac } from 'node:crypto'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { issueTicket } from './ticket'

const SECRET = 'test-live-secret'
const NOW = new Date('2026-09-12T12:00:00Z')

function decodePayload(ticket: string): unknown {
  const [body] = ticket.split('.')
  return JSON.parse(Buffer.from(body, 'base64url').toString('utf8'))
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(NOW)
  vi.stubEnv('VPN_LIVE_SECRET', SECRET)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllEnvs()
})

describe('issueTicket', () => {
  it('throws when VPN_LIVE_SECRET is missing or empty', () => {
    vi.stubEnv('VPN_LIVE_SECRET', undefined)
    expect(() => issueTicket('admin', 'admin')).toThrow('VPN_LIVE_SECRET')

    vi.stubEnv('VPN_LIVE_SECRET', '')
    expect(() => issueTicket('admin', 'admin')).toThrow('VPN_LIVE_SECRET')
  })

  it('is two unpadded base64url parts joined by a dot', () => {
    expect(issueTicket('admin', 'admin')).toMatch(/^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/)
  })

  it('carries uid, role and an expiry five minutes from now', () => {
    const expiry = Math.floor(NOW.getTime() / 1000) + 5 * 60

    expect(decodePayload(issueTicket('user-1', 'admin'))).toEqual({
      uid: 'user-1',
      role: 'admin',
      exp: expiry,
    })
  })

  it('signs the encoded payload with HMAC-SHA256 of VPN_LIVE_SECRET', () => {
    const [body, signature] = issueTicket('admin', 'admin').split('.')

    expect(signature).toBe(createHmac('sha256', SECRET).update(body).digest('base64url'))
    expect(signature).not.toBe(createHmac('sha256', 'other').update(body).digest('base64url'))
  })

  it('reads the secret on every call', () => {
    const [, first] = issueTicket('admin', 'admin').split('.')
    vi.stubEnv('VPN_LIVE_SECRET', 'rotated')
    const [, second] = issueTicket('admin', 'admin').split('.')

    expect(second).not.toBe(first)
  })
})
