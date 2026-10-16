import { getCloudflareContext } from '@opennextjs/cloudflare'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  clearFailedLogins,
  clientAddress,
  LOCKOUT_MINUTES,
  loginBlocked,
  MAX_FAILED_LOGINS,
  recordFailedLogin,
} from './login-attempts'

vi.mock('@opennextjs/cloudflare', () => ({ getCloudflareContext: vi.fn() }))

const NOW = new Date('2026-09-17T12:00:00Z')
const nowSeconds = NOW.getTime() / 1000

const kv = { get: vi.fn(), put: vi.fn(), delete: vi.fn() }

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(NOW)
  vi.spyOn(console, 'error').mockImplementation(() => {})
  vi.mocked(getCloudflareContext).mockReturnValue({
    env: { LOGIN_ATTEMPTS: kv },
  } as unknown as ReturnType<typeof getCloudflareContext>)
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  kv.get.mockReset()
  kv.put.mockReset()
  kv.delete.mockReset()
})

describe('clientAddress', () => {
  it.each([
    [{ 'cf-connecting-ip': '203.0.113.7', 'x-forwarded-for': '198.51.100.1' }, '203.0.113.7'],
    [{ 'x-forwarded-for': '203.0.113.5, 10.0.0.1' }, '203.0.113.5'],
    [{ 'x-real-ip': '203.0.113.6' }, '203.0.113.6'],
    [{}, 'unknown'],
  ])('reads %j as %s', (headers, expected) => {
    expect(clientAddress(new Request('https://panel.test', { headers }))).toBe(expected)
  })
})

describe('loginBlocked', () => {
  it('blocks an address with the maximum of failures in an open window', async () => {
    kv.get.mockResolvedValue({ attempts: MAX_FAILED_LOGINS, expiresAt: nowSeconds + 60 })

    expect(await loginBlocked('203.0.113.5')).toBe(true)
    expect(kv.get).toHaveBeenCalledWith('login:203.0.113.5', 'json')
  })

  it.each([
    ['below the maximum', { attempts: MAX_FAILED_LOGINS - 1, expiresAt: nowSeconds + 60 }],
    [
      'whose window ended but is still stored',
      { attempts: MAX_FAILED_LOGINS, expiresAt: nowSeconds },
    ],
    ['without a window', null],
  ])('lets through an address %s', async (_, stored) => {
    kv.get.mockResolvedValue(stored)
    expect(await loginBlocked('203.0.113.5')).toBe(false)
  })

  it('fails open when KV does not answer', async () => {
    kv.get.mockRejectedValue(new Error('kv down'))
    expect(await loginBlocked('203.0.113.5')).toBe(false)
  })
})

describe('recordFailedLogin', () => {
  it('counts a failure in the open window without extending it', async () => {
    const expiresAt = nowSeconds + 600
    kv.get.mockResolvedValue({ attempts: 3, expiresAt })

    await recordFailedLogin('203.0.113.5')

    expect(kv.put).toHaveBeenCalledWith(
      'login:203.0.113.5',
      JSON.stringify({ attempts: 4, expiresAt }),
      { expiration: expiresAt },
    )
  })

  it(`opens a ${LOCKOUT_MINUTES}-minute window when none is open`, async () => {
    kv.get.mockResolvedValue(null)

    await recordFailedLogin('203.0.113.5')

    const expiresAt = nowSeconds + LOCKOUT_MINUTES * 60
    expect(kv.put).toHaveBeenCalledWith(
      'login:203.0.113.5',
      JSON.stringify({ attempts: 1, expiresAt }),
      { expiration: expiresAt },
    )
  })

  it('keeps the key at least a minute, the least KV accepts', async () => {
    kv.get.mockResolvedValue({ attempts: 2, expiresAt: nowSeconds + 10 })

    await recordFailedLogin('203.0.113.5')

    expect(kv.put.mock.calls[0][2]).toEqual({ expiration: nowSeconds + 60 })
  })

  it('never throws', async () => {
    kv.get.mockRejectedValue(new Error('kv down'))
    await expect(recordFailedLogin('203.0.113.5')).resolves.toBeUndefined()
  })
})

describe('clearFailedLogins', () => {
  it('drops the address', async () => {
    await clearFailedLogins('203.0.113.5')
    expect(kv.delete).toHaveBeenCalledWith('login:203.0.113.5')
  })
})
