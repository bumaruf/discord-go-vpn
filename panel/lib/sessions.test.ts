import { ObjectId } from 'mongodb'
import { beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest'
import { fakeCollection, fakeCursor } from './fake-collection'
import { collection } from './mongodb'
import { findSessionsUpdatedSince, type SessionDocument } from './sessions'

vi.mock('./mongodb', () => ({ collection: vi.fn() }))

const sessions = fakeCollection()
const INVITE_ID = new ObjectId('66e2f0a1b2c3d4e5f6a7b8c9')

beforeEach(() => {
  sessions.find.mockReset()
  vi.mocked(collection).mockResolvedValue(sessions.asCollection())
})

describe('findSessionsUpdatedSince', () => {
  it('reads up to 200 sessions updated since the cutoff', async () => {
    const cursor = fakeCursor([])
    sessions.find.mockReturnValue(cursor)
    const since = new Date('2026-09-12T10:03:00Z')

    await findSessionsUpdatedSince(since)

    expect(collection).toHaveBeenCalledWith('sessions')
    expect(sessions.find).toHaveBeenCalledWith({ updatedAt: { $gte: since } })
    expect(cursor.limit).toHaveBeenCalledWith(200)
    expect(cursor.sort).not.toHaveBeenCalled()
  })

  it('maps the stored document to a session', async () => {
    sessions.find.mockReturnValue(
      fakeCursor([
        {
          _id: 'lease-1',
          inviteId: INVITE_ID,
          deviceId: 'device-1',
          label: 'desktop',
          pubKey: 'pub-key',
          ip: '10.9.0.2',
          originIp: '203.0.113.7',
          startedAt: new Date('2026-09-12T10:00:00Z'),
          lastHandshake: new Date('2026-09-12T10:05:00Z'),
          updatedAt: new Date('2026-09-12T10:05:01Z'),
          rxBytes: 1_000,
          txBytes: 2_000,
          status: 'online',
        },
        { _id: 'lease-2', inviteId: null, status: 'offline' },
      ]),
    )

    expect(await findSessionsUpdatedSince(new Date())).toEqual([
      {
        leaseId: 'lease-1',
        inviteId: INVITE_ID.toHexString(),
        deviceId: 'device-1',
        label: 'desktop',
        publicKey: 'pub-key',
        ip: '10.9.0.2',
        originIp: '203.0.113.7',
        lastHandshake: new Date('2026-09-12T10:05:00Z'),
        rxBytes: 1_000,
        txBytes: 2_000,
        online: true,
      },
      {
        leaseId: 'lease-2',
        inviteId: undefined,
        deviceId: undefined,
        label: undefined,
        publicKey: undefined,
        ip: undefined,
        originIp: undefined,
        lastHandshake: undefined,
        rxBytes: undefined,
        txBytes: undefined,
        online: false,
      },
    ])
  })
})

// Checked by `npm run typecheck`: the session document only declares what provisiond writes.
describe('SessionDocument', () => {
  it('has no field the collector never writes', () => {
    expectTypeOf<
      Extract<keyof SessionDocument, 'rtt' | 'loss' | 'city' | 'country' | 'endedAt'>
    >().toBeNever()
    expectTypeOf<SessionDocument['status']>().toEqualTypeOf<'online' | 'offline'>()
  })
})
