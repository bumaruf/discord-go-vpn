import { ObjectId } from 'mongodb'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { countDevicesByInvite, listDevices } from './devices'
import { fakeCollection, fakeCursor } from './fake-collection'
import { collection } from './mongodb'

vi.mock('./mongodb', () => ({ collection: vi.fn() }))

const devices = fakeCollection()
const FIRST_ID = new ObjectId('66e2f0a1b2c3d4e5f6a7b8c9')
const SECOND_ID = new ObjectId('66e2f0a1b2c3d4e5f6a7b8ca')
const INVITE_ID = new ObjectId('66e2f0a1b2c3d4e5f6a7b8cb')

beforeEach(() => {
  vi.mocked(collection).mockClear()
  devices.aggregate.mockReset()
  devices.find.mockReset()
  vi.mocked(collection).mockResolvedValue(devices.asCollection())
})

describe('listDevices', () => {
  it('returns every machine, the most recently seen first, with dates as ISO strings', async () => {
    const cursor = fakeCursor([
      {
        _id: FIRST_ID,
        inviteId: INVITE_ID,
        name: 'laptop',
        osUser: 'ana',
        lastIp: '198.51.100.7',
        createdAt: new Date('2026-08-01T10:00:00Z'),
        lastSeenAt: new Date('2026-09-19T10:00:00Z'),
        accessCount: 7,
      },
    ])
    devices.find.mockReturnValue(cursor)

    const listed = await listDevices()

    expect(devices.find).toHaveBeenCalledWith({})
    expect(cursor.sort).toHaveBeenCalledWith({ lastSeenAt: -1 })
    expect(listed).toEqual([
      {
        id: FIRST_ID.toHexString(),
        inviteId: INVITE_ID.toHexString(),
        name: 'laptop',
        osUser: 'ana',
        lastIp: '198.51.100.7',
        firstSeenAt: '2026-08-01T10:00:00.000Z',
        lastSeenAt: '2026-09-19T10:00:00.000Z',
        accessCount: 7,
      },
    ])
  })

  it('leaves out the invite and the os user of a device that has none', async () => {
    devices.find.mockReturnValue(
      fakeCursor([
        {
          _id: FIRST_ID,
          inviteId: null,
          name: '',
          osUser: null,
          createdAt: new Date('2026-08-01T10:00:00Z'),
          lastSeenAt: new Date('2026-08-02T10:00:00Z'),
          accessCount: 1,
        },
      ]),
    )

    expect(await listDevices()).toEqual([
      {
        id: FIRST_ID.toHexString(),
        inviteId: undefined,
        name: '',
        osUser: undefined,
        lastIp: undefined,
        firstSeenAt: '2026-08-01T10:00:00.000Z',
        lastSeenAt: '2026-08-02T10:00:00.000Z',
        accessCount: 1,
      },
    ])
  })
})

describe('countDevicesByInvite', () => {
  it('counts devices per invite in one aggregation', async () => {
    devices.aggregate.mockReturnValue({
      toArray: vi.fn(async () => [{ _id: FIRST_ID, total: 3 }]),
    })

    const counts = await countDevicesByInvite([FIRST_ID.toHexString(), SECOND_ID.toHexString()])

    expect(collection).toHaveBeenCalledWith('devices')
    expect(devices.aggregate).toHaveBeenCalledWith([
      { $match: { inviteId: { $in: [FIRST_ID, SECOND_ID] } } },
      { $group: { _id: '$inviteId', total: { $sum: 1 } } },
    ])
    expect(counts).toEqual(new Map([[FIRST_ID.toHexString(), 3]]))
  })

  it('skips the database without invites', async () => {
    expect(await countDevicesByInvite([])).toEqual(new Map())
    expect(collection).not.toHaveBeenCalled()
  })
})
