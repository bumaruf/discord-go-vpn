import { ObjectId, type Collection, type Db } from 'mongodb'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mergeDevices, planDeviceMerges } from './merge-devices'
import { COLLECTION } from './mongo-names'

function fakeCursor<T>(docs: T[]) {
  const cursor = { sort: vi.fn(), limit: vi.fn(), toArray: vi.fn(async () => docs) }
  cursor.sort.mockReturnValue(cursor)
  cursor.limit.mockReturnValue(cursor)
  return cursor
}

function fakeCollection() {
  const fake = {
    find: vi.fn(),
    findOne: vi.fn(),
    aggregate: vi.fn(),
    insertOne: vi.fn(),
    updateOne: vi.fn(),
    updateMany: vi.fn(),
    deleteOne: vi.fn(),
  }
  return { ...fake, asCollection: () => fake as unknown as Collection }
}

const OLD = new ObjectId('66e2f0a1b2c3d4e5f6a7b8c9')
const CURRENT = new ObjectId('66e2f0a1b2c3d4e5f6a7b8ca')
const OTHER = new ObjectId('66e2f0a1b2c3d4e5f6a7b8cb')
const INVITE = new ObjectId('66e2f0a1b2c3d4e5f6a7b8cc')
const NEW_INVITE = new ObjectId('66e2f0a1b2c3d4e5f6a7b8cd')
const FIRST_ACCESS = new Date('2026-08-01T10:00:00Z')
const LAST_ACCESS = new Date('2026-10-01T10:00:00Z')

const devices = fakeCollection()
const dailyUsage = fakeCollection()
const sessions = fakeCollection()

const collections: Record<string, ReturnType<typeof fakeCollection>> = {
  [COLLECTION.devices]: devices,
  [COLLECTION.dailyUsage]: dailyUsage,
  [COLLECTION.sessions]: sessions,
}

const db = { collection: (name: string) => collections[name].asCollection() } as unknown as Db

function device(
  id: ObjectId,
  inviteId: ObjectId | null,
  osUser: string | null,
  { name = 'laptop', lastSeenAt = FIRST_ACCESS } = {},
) {
  return {
    _id: id,
    inviteId,
    name,
    osUser,
    firstIp: '198.51.100.7',
    createdAt: FIRST_ACCESS,
    lastSeenAt,
    accessCount: 4,
  }
}

function withDevices(docs: unknown[]) {
  devices.find.mockReturnValue(fakeCursor(docs))
}

async function planIds() {
  const plan = await planDeviceMerges(db)
  return {
    machines: plan.machines.map(({ into, from }) => ({
      into: into._id,
      from: from.map((d) => d._id),
    })),
    ambiguous: plan.ambiguous.map(({ device, osUsers }) => ({ id: device._id, osUsers })),
  }
}

beforeEach(() => {
  for (const fake of Object.values(collections)) {
    for (const method of Object.values(fake)) {
      if (typeof method === 'function' && 'mockReset' in method) method.mockReset()
    }
  }
  dailyUsage.find.mockReturnValue(fakeCursor([]))
  sessions.updateMany.mockResolvedValue({ modifiedCount: 0 })
})

describe('planDeviceMerges', () => {
  it('joins one machine across invites, keeping the device seen last', async () => {
    withDevices([
      device(OLD, INVITE, 'ana'),
      device(CURRENT, NEW_INVITE, 'ana', { lastSeenAt: LAST_ACCESS }),
      device(OTHER, null, 'ana'),
    ])

    expect(await planIds()).toEqual({
      machines: [{ into: CURRENT, from: [OLD, OTHER] }],
      ambiguous: [],
    })
  })

  it('ignores case in the name and the os user', async () => {
    withDevices([
      device(OLD, INVITE, 'Ana', { name: 'LAPTOP' }),
      device(CURRENT, INVITE, 'ana', { lastSeenAt: LAST_ACCESS }),
    ])

    expect((await planIds()).machines).toEqual([{ into: CURRENT, from: [OLD] }])
  })

  it('places a device without an os user with the only os user of its name', async () => {
    withDevices([device(OLD, INVITE, null), device(CURRENT, NEW_INVITE, 'ana')])

    expect((await planIds()).machines).toEqual([{ into: CURRENT, from: [OLD] }])
  })

  it('leaves a device without an os user alone when its name has several', async () => {
    withDevices([
      device(OLD, INVITE, null),
      device(CURRENT, INVITE, 'ana'),
      device(OTHER, INVITE, 'root'),
    ])

    expect(await planIds()).toEqual({
      machines: [],
      ambiguous: [{ id: OLD, osUsers: ['ana', 'root'] }],
    })
  })

  it('keeps different os users and unnamed machines apart', async () => {
    withDevices([
      device(OLD, INVITE, 'ana'),
      device(CURRENT, INVITE, 'root'),
      device(OTHER, INVITE, 'ana', { name: '' }),
      device(new ObjectId(), NEW_INVITE, 'ana', { name: '' }),
    ])

    expect(await planIds()).toEqual({ machines: [], ambiguous: [] })
  })
})

describe('mergeDevices', () => {
  it('moves the history of each device into the one that stays', async () => {
    dailyUsage.find.mockReturnValue(
      fakeCursor([
        {
          _id: `${OLD.toHexString()}:2026-08-30`,
          day: '2026-08-30',
          rxBytes: 10,
          txBytes: 2,
          onlineSeconds: 60,
        },
        {
          _id: `${OLD.toHexString()}:2026-08-31`,
          day: '2026-08-31',
          rxBytes: 5,
          txBytes: 1,
          onlineSeconds: 30,
        },
      ]),
    )
    sessions.updateMany.mockResolvedValue({ modifiedCount: 3 })

    const merged = await mergeDevices(db, {
      machines: [{ into: device(CURRENT, NEW_INVITE, 'ana'), from: [device(OLD, INVITE, 'ana')] }],
      ambiguous: [],
    })

    expect(merged).toEqual([
      {
        from: OLD.toHexString(),
        into: CURRENT.toHexString(),
        name: 'laptop',
        osUser: 'ana',
        days: 2,
        sessions: 3,
      },
    ])
  })

  it('deletes each day before adding it, so an interrupted run never counts one twice', async () => {
    dailyUsage.find.mockReturnValue(
      fakeCursor([
        {
          _id: `${OLD.toHexString()}:2026-08-30`,
          day: '2026-08-30',
          rxBytes: 10,
          txBytes: 2,
          onlineSeconds: 60,
        },
      ]),
    )

    await mergeDevices(db, {
      machines: [{ into: device(CURRENT, INVITE, 'ana'), from: [device(OLD, null, 'ana')] }],
      ambiguous: [],
    })

    expect(dailyUsage.deleteOne.mock.invocationCallOrder[0]).toBeLessThan(
      dailyUsage.updateOne.mock.invocationCallOrder[0],
    )
    expect(dailyUsage.deleteOne).toHaveBeenCalledWith({ _id: `${OLD.toHexString()}:2026-08-30` })
    expect(dailyUsage.updateOne).toHaveBeenCalledWith(
      { _id: `${CURRENT.toHexString()}:2026-08-30` },
      {
        $inc: { rxBytes: 10, txBytes: 2, onlineSeconds: 60 },
        $setOnInsert: { identity: CURRENT.toHexString(), day: '2026-08-30' },
      },
      { upsert: true },
    )
  })

  it('re-points the sessions, carries the first access over and deletes the merged device', async () => {
    await mergeDevices(db, {
      machines: [{ into: device(CURRENT, INVITE, 'ana'), from: [device(OLD, null, 'ana')] }],
      ambiguous: [],
    })

    expect(sessions.updateMany).toHaveBeenCalledWith(
      { deviceId: OLD.toHexString() },
      { $set: { deviceId: CURRENT.toHexString() } },
    )
    expect(devices.updateOne).toHaveBeenCalledWith(
      { _id: CURRENT },
      {
        $min: { createdAt: FIRST_ACCESS },
        $inc: { accessCount: 4 },
        $set: { firstIp: '198.51.100.7' },
      },
    )
    expect(devices.deleteOne).toHaveBeenCalledWith({ _id: OLD })
  })

  it('gives the device that stays the os user it lacked, after the others are gone', async () => {
    await mergeDevices(db, {
      machines: [{ into: device(CURRENT, INVITE, null), from: [device(OLD, INVITE, 'ana')] }],
      ambiguous: [],
    })

    expect(devices.updateOne).toHaveBeenLastCalledWith(
      { _id: CURRENT },
      { $set: { osUser: 'ana' } },
    )
    expect(devices.deleteOne.mock.invocationCallOrder[0]).toBeLessThan(
      devices.updateOne.mock.invocationCallOrder.at(-1)!,
    )
  })
})
