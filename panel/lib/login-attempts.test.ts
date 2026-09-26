import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fakeCollection } from './fake-collection'
import {
  clearFailedLogins,
  clientAddress,
  loginBlocked,
  MAX_FAILED_LOGINS,
  recordFailedLogin,
} from './login-attempts'
import { collection } from './mongodb'

vi.mock('./mongodb', () => ({ collection: vi.fn() }))

const attempts = fakeCollection()
const NOW = new Date('2026-09-17T12:00:00Z')

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(NOW)
  vi.spyOn(console, 'error').mockImplementation(() => {})
  vi.mocked(collection).mockResolvedValue(attempts.asCollection())
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  attempts.findOne.mockReset()
  attempts.updateOne.mockReset()
  attempts.deleteOne.mockReset()
})

describe('clientAddress', () => {
  it.each([
    [{ 'x-forwarded-for': '203.0.113.5, 10.0.0.1' }, '203.0.113.5'],
    [{ 'x-real-ip': '203.0.113.6' }, '203.0.113.6'],
    [{}, 'unknown'],
  ])('reads %j as %s', (headers, expected) => {
    expect(clientAddress(new Request('https://panel.test', { headers }))).toBe(expected)
  })
})

describe('loginBlocked', () => {
  it('blocks an address with the maximum of failures in an open window', async () => {
    attempts.findOne.mockResolvedValue({ _id: '203.0.113.5', attempts: MAX_FAILED_LOGINS })

    expect(await loginBlocked('203.0.113.5')).toBe(true)
    expect(collection).toHaveBeenCalledWith('login_attempts')
    expect(attempts.findOne).toHaveBeenCalledWith({ _id: '203.0.113.5', expiresAt: { $gt: NOW } })
  })

  it('lets through an address below the maximum or without a window', async () => {
    attempts.findOne.mockResolvedValue({ _id: '203.0.113.5', attempts: MAX_FAILED_LOGINS - 1 })
    expect(await loginBlocked('203.0.113.5')).toBe(false)

    attempts.findOne.mockResolvedValue(null)
    expect(await loginBlocked('203.0.113.5')).toBe(false)
  })

  it('fails open when the database does not answer', async () => {
    attempts.findOne.mockRejectedValue(new Error('down'))

    expect(await loginBlocked('203.0.113.5')).toBe(false)
  })
})

describe('recordFailedLogin', () => {
  it('counts a failure in the open window', async () => {
    attempts.updateOne.mockResolvedValue({ matchedCount: 1 })

    await recordFailedLogin('203.0.113.5')

    expect(attempts.updateOne).toHaveBeenCalledTimes(1)
    expect(attempts.updateOne).toHaveBeenCalledWith(
      { _id: '203.0.113.5', expiresAt: { $gt: NOW } },
      { $inc: { attempts: 1 } },
    )
  })

  it('opens a 15-minute window when none is open', async () => {
    attempts.updateOne.mockResolvedValueOnce({ matchedCount: 0 })

    await recordFailedLogin('203.0.113.5')

    expect(attempts.updateOne).toHaveBeenLastCalledWith(
      { _id: '203.0.113.5' },
      { $set: { attempts: 1, expiresAt: new Date('2026-09-17T12:15:00Z') } },
      { upsert: true },
    )
  })

  it('never throws', async () => {
    attempts.updateOne.mockRejectedValue(new Error('down'))

    await expect(recordFailedLogin('203.0.113.5')).resolves.toBeUndefined()
  })
})

describe('clearFailedLogins', () => {
  it('drops the address', async () => {
    await clearFailedLogins('203.0.113.5')

    expect(attempts.deleteOne).toHaveBeenCalledWith({ _id: '203.0.113.5' })
  })
})
