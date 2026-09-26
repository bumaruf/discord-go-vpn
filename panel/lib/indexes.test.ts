import type { Db } from 'mongodb'
import { describe, expect, it, vi } from 'vitest'
import { ensureIndexes } from './indexes'

type FakeCollection = { createIndex: ReturnType<typeof vi.fn> }

function fakeDb(samplesExist: boolean) {
  const collections = new Map<string, FakeCollection>()
  const database = {
    collection: vi.fn((name: string) => {
      if (!collections.has(name)) collections.set(name, { createIndex: vi.fn() })
      return collections.get(name)!
    }),
    listCollections: vi.fn(() => ({ hasNext: vi.fn(async () => samplesExist) })),
    createCollection: vi.fn(),
  }
  const indexes = (name: string) => collections.get(name)?.createIndex.mock.calls
  return { database, indexes }
}

describe('ensureIndexes', () => {
  it('creates the same indexes on the same collections', async () => {
    const { database, indexes } = fakeDb(true)

    const lines = await ensureIndexes(database as unknown as Db)

    expect(lines).toEqual([
      'invites.tokenHash (unique), invites.status+createdAt',
      'devices.inviteId+name+osUser (unique), devices.lastSeenAt',
      'sessions.inviteId+updatedAt, sessions.deviceId+startedAt',
      'daily_usage.identity+day',
      'audit_log.at',
      'login_attempts.expiresAt (TTL)',
      'samples (already exists)',
    ])
    expect(indexes('invites')).toEqual([
      [{ tokenHash: 1 }, { unique: true }],
      [{ status: 1, createdAt: -1 }],
    ])
    expect(indexes('devices')).toEqual([
      [{ inviteId: 1, name: 1, osUser: 1 }, { unique: true }],
      [{ lastSeenAt: -1 }],
    ])
    expect(indexes('sessions')).toEqual([
      [{ inviteId: 1, updatedAt: -1 }],
      [{ deviceId: 1, startedAt: -1 }],
      [{ updatedAt: -1 }],
    ])
    expect(indexes('daily_usage')).toEqual([[{ identity: 1, day: -1 }]])
    expect(indexes('audit_log')).toEqual([[{ at: -1 }]])
    expect(indexes('login_attempts')).toEqual([[{ expiresAt: 1 }, { expireAfterSeconds: 0 }]])
    expect(database.listCollections).toHaveBeenCalledWith({ name: 'samples' })
    expect(database.createCollection).not.toHaveBeenCalled()
  })

  it('creates the samples time series with a 48h TTL when it is missing', async () => {
    const { database } = fakeDb(false)

    const lines = await ensureIndexes(database as unknown as Db)

    expect(lines.at(-1)).toBe('samples (time series, TTL 48h)')
    expect(database.createCollection).toHaveBeenCalledWith('samples', {
      timeseries: { timeField: 'ts', metaField: 'meta', granularity: 'seconds' },
      expireAfterSeconds: 48 * 60 * 60,
    })
  })
})
