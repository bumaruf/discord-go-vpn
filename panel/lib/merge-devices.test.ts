import { ObjectId, type Db } from 'mongodb'
import { beforeEach, describe, expect, it } from 'vitest'
import { fakeCollection, fakeCursor } from './fake-collection'
import { mergeDevicesWithoutInvite } from './merge-devices'
import { COLLECTION } from './mongo-names'

const ORPHAN = new ObjectId('66e2f0a1b2c3d4e5f6a7b8c9')
const CURRENT = new ObjectId('66e2f0a1b2c3d4e5f6a7b8ca')
const OTHER = new ObjectId('66e2f0a1b2c3d4e5f6a7b8cb')
const INVITE = new ObjectId('66e2f0a1b2c3d4e5f6a7b8cc')
const FIRST_ACCESS = new Date('2026-08-01T10:00:00Z')

const devices = fakeCollection()
const dailyUsage = fakeCollection()
const sessions = fakeCollection()

const collections: Record<string, ReturnType<typeof fakeCollection>> = {
  [COLLECTION.devices]: devices,
  [COLLECTION.dailyUsage]: dailyUsage,
  [COLLECTION.sessions]: sessions,
}

const db = { collection: (name: string) => collections[name].asCollection() } as unknown as Db

function device(id: ObjectId, inviteId: ObjectId | null, osUser: string | null) {
  return {
    _id: id,
    inviteId,
    name: 'laptop',
    osUser,
    firstIp: '198.51.100.7',
    createdAt: FIRST_ACCESS,
    lastSeenAt: FIRST_ACCESS,
    accessCount: 4,
  }
}

/** The orphans come from the first find; every other find is looking for their candidates. */
function withDevices(orphans: unknown[], candidates: unknown[]) {
  devices.find.mockImplementation((filter: Record<string, unknown>) =>
    fakeCursor(filter.inviteId === null ? orphans : candidates),
  )
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

describe('mergeDevicesWithoutInvite', () => {
  it('moves the history of a device without an invite into the machine current device', async () => {
    withDevices([device(ORPHAN, null, 'ana')], [device(CURRENT, INVITE, 'ana')])
    dailyUsage.find.mockReturnValue(
      fakeCursor([
        {
          _id: `${ORPHAN.toHexString()}:2026-08-30`,
          day: '2026-08-30',
          rxBytes: 10,
          txBytes: 2,
          onlineSeconds: 60,
        },
        {
          _id: `${ORPHAN.toHexString()}:2026-08-31`,
          day: '2026-08-31',
          rxBytes: 5,
          txBytes: 1,
          onlineSeconds: 30,
        },
      ]),
    )
    sessions.updateMany.mockResolvedValue({ modifiedCount: 3 })

    const report = await mergeDevicesWithoutInvite(db)

    expect(report.merged).toEqual([
      {
        from: ORPHAN.toHexString(),
        into: CURRENT.toHexString(),
        name: 'laptop',
        osUser: 'ana',
        days: 2,
        sessions: 3,
      },
    ])
    expect(report.waiting).toEqual([])
    expect(report.ambiguous).toEqual([])
  })

  it('deletes each day before adding it, so an interrupted run never counts one twice', async () => {
    withDevices([device(ORPHAN, null, 'ana')], [device(CURRENT, INVITE, 'ana')])
    dailyUsage.find.mockReturnValue(
      fakeCursor([
        {
          _id: `${ORPHAN.toHexString()}:2026-08-30`,
          day: '2026-08-30',
          rxBytes: 10,
          txBytes: 2,
          onlineSeconds: 60,
        },
      ]),
    )

    await mergeDevicesWithoutInvite(db)

    expect(dailyUsage.deleteOne.mock.invocationCallOrder[0]).toBeLessThan(
      dailyUsage.updateOne.mock.invocationCallOrder[0],
    )
    expect(dailyUsage.deleteOne).toHaveBeenCalledWith({ _id: `${ORPHAN.toHexString()}:2026-08-30` })
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
    withDevices([device(ORPHAN, null, 'ana')], [device(CURRENT, INVITE, 'ana')])

    await mergeDevicesWithoutInvite(db)

    expect(sessions.updateMany).toHaveBeenCalledWith(
      { deviceId: ORPHAN.toHexString() },
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
    expect(devices.deleteOne).toHaveBeenCalledWith({ _id: ORPHAN })
  })

  it('looks the machine up by a null os user, which also matches a missing one', async () => {
    withDevices([{ ...device(ORPHAN, null, null), osUser: undefined }], [])

    await mergeDevicesWithoutInvite(db)

    expect(devices.find).toHaveBeenLastCalledWith({
      name: 'laptop',
      osUser: null,
      inviteId: { $ne: null },
    })
  })

  it('leaves a device whose machine never came back for provisiond to adopt', async () => {
    withDevices([device(ORPHAN, null, 'ana')], [])

    const report = await mergeDevicesWithoutInvite(db)

    expect(report.merged).toEqual([])
    expect(report.waiting).toEqual([
      { id: ORPHAN.toHexString(), name: 'laptop', osUser: 'ana', candidates: [] },
    ])
    expect(devices.deleteOne).not.toHaveBeenCalled()
  })

  it('leaves a device that matches more than one invite for an admin to decide', async () => {
    withDevices(
      [device(ORPHAN, null, 'ana')],
      [device(CURRENT, INVITE, 'ana'), device(OTHER, INVITE, 'ana')],
    )

    const report = await mergeDevicesWithoutInvite(db)

    expect(report.merged).toEqual([])
    expect(report.ambiguous).toEqual([
      {
        id: ORPHAN.toHexString(),
        name: 'laptop',
        osUser: 'ana',
        candidates: [CURRENT.toHexString(), OTHER.toHexString()],
      },
    ])
    expect(devices.deleteOne).not.toHaveBeenCalled()
  })

  it('does nothing when no device is without an invite', async () => {
    withDevices([], [])

    expect(await mergeDevicesWithoutInvite(db)).toEqual({
      merged: [],
      waiting: [],
      ambiguous: [],
    })
  })
})
