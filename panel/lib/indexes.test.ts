import type { Db } from 'mongodb'
import { describe, expect, it, vi } from 'vitest'
import { ensureIndexes } from './indexes'

type FakeCollection = {
  createIndex: ReturnType<typeof vi.fn>
  indexExists: ReturnType<typeof vi.fn>
  dropIndex: ReturnType<typeof vi.fn>
}

function fakeDb(samplesExist: boolean, oldDeviceIndex = false) {
  const collections = new Map<string, FakeCollection>()
  const database = {
    collection: vi.fn((name: string) => {
      if (!collections.has(name)) {
        collections.set(name, {
          createIndex: vi.fn(),
          indexExists: vi.fn(async () => oldDeviceIndex),
          dropIndex: vi.fn(),
        })
      }
      return collections.get(name)!
    }),
    listCollections: vi.fn(() => ({ hasNext: vi.fn(async () => samplesExist) })),
    createCollection: vi.fn(),
  }
  const indexes = (name: string) => collections.get(name)?.createIndex.mock.calls
  const devices = () => database.collection('devices')
  return { database, indexes, devices }
}

describe('ensureIndexes', () => {
  it('creates the same indexes on the same collections', async () => {
    const { database, indexes } = fakeDb(true)

    const lines = await ensureIndexes(database as unknown as Db)

    expect(lines).toEqual([
      'invites.tokenHash (unique), invites.status+createdAt',
      'devices.inviteId+name+osUser (unique, case-insensitive), devices.lastSeenAt',
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
      [
        { inviteId: 1, name: 1, osUser: 1 },
        {
          unique: true,
          collation: { locale: 'en', strength: 2 },
          name: 'device_key_case_insensitive',
        },
      ],
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

  it('drops the case-sensitive device index only after creating the case-insensitive one', async () => {
    const { database, devices } = fakeDb(true, true)

    await ensureIndexes(database as unknown as Db)

    expect(devices().dropIndex).toHaveBeenCalledWith('inviteId_1_name_1_osUser_1')
    expect(devices().createIndex.mock.invocationCallOrder[0]).toBeLessThan(
      devices().dropIndex.mock.invocationCallOrder[0],
    )
  })

  it('keeps the case-sensitive index and asks for a merge when case variants collide', async () => {
    const { database, devices } = fakeDb(true, true)
    devices().createIndex.mockRejectedValueOnce(Object.assign(new Error('E11000'), { code: 11000 }))

    await expect(ensureIndexes(database as unknown as Db)).rejects.toThrow(/merge-devices/)
    expect(devices().dropIndex).not.toHaveBeenCalled()
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
