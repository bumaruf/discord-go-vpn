import { ObjectId } from 'mongodb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fakeCollection, fakeCursor } from './fake-collection'
import {
  deleteInvite,
  insertInvite,
  inviteLabelsById,
  listRecentInvites,
  setInviteMaxLeases,
} from './invites'
import { collection } from './mongodb'

vi.mock('./mongodb', () => ({ collection: vi.fn() }))

const invites = fakeCollection()
const FIRST_ID = new ObjectId('66e2f0a1b2c3d4e5f6a7b8c9')
const SECOND_ID = new ObjectId('66e2f0a1b2c3d4e5f6a7b8ca')

beforeEach(() => {
  invites.find.mockReset()
  invites.insertOne.mockReset()
  invites.deleteOne.mockReset()
  invites.updateOne.mockReset()
  vi.mocked(collection).mockResolvedValue(invites.asCollection())
})

afterEach(() => {
  vi.useRealTimers()
})

describe('listRecentInvites', () => {
  it('reads the 100 newest invites', async () => {
    const cursor = fakeCursor([])
    invites.find.mockReturnValue(cursor)

    await listRecentInvites()

    expect(collection).toHaveBeenCalledWith('invites')
    expect(invites.find).toHaveBeenCalledWith({})
    expect(cursor.sort).toHaveBeenCalledWith({ createdAt: -1 })
    expect(cursor.limit).toHaveBeenCalledWith(100)
  })

  it('maps each stored invite', async () => {
    const createdAt = new Date('2026-09-12T10:00:00Z')
    invites.find.mockReturnValue(
      fakeCursor([
        {
          _id: FIRST_ID,
          label: 'Team A',
          kind: 'global',
          prefix: 'vpn_ab12',
          status: 'active',
          createdAt,
          maxLeases: 4,
        },
        {
          _id: SECOND_ID,
          label: 'friends',
          prefix: 'vpn_cd34',
          status: 'revoked',
          createdAt,
        },
      ]),
    )

    expect(await listRecentInvites()).toEqual([
      {
        id: FIRST_ID.toHexString(),
        label: 'Team A',
        kind: 'global',
        prefix: 'vpn_ab12',
        active: true,
        createdAt,
        maxLeases: 4,
      },
      {
        id: SECOND_ID.toHexString(),
        label: 'friends',
        kind: 'individual',
        prefix: 'vpn_cd34',
        active: false,
        createdAt,
        maxLeases: null,
      },
    ])
  })
})

describe('inviteLabelsById', () => {
  it('reads only the label of up to 200 invites, keyed by id', async () => {
    const cursor = fakeCursor([
      { _id: FIRST_ID, label: 'Team A' },
      { _id: SECOND_ID, label: 'friends' },
    ])
    invites.find.mockReturnValue(cursor)

    const labels = await inviteLabelsById()

    expect(invites.find).toHaveBeenCalledWith({}, { projection: { label: 1 } })
    expect(cursor.limit).toHaveBeenCalledWith(200)
    expect(labels).toEqual({
      [FIRST_ID.toHexString()]: 'Team A',
      [SECOND_ID.toHexString()]: 'friends',
    })
  })
})

describe('insertInvite', () => {
  it('stores an active invite created now, in the stored field order', async () => {
    const now = new Date('2026-09-13T12:00:00Z')
    vi.useFakeTimers()
    vi.setSystemTime(now)

    await insertInvite({
      label: 'Team A',
      kind: 'global',
      tokenHash: 'hash',
      prefix: 'vpn_ab12',
      createdBy: 'admin',
      maxLeases: null,
    })

    const [document] = invites.insertOne.mock.calls[0]
    expect(document).toEqual({
      _id: expect.any(ObjectId),
      label: 'Team A',
      kind: 'global',
      tokenHash: 'hash',
      prefix: 'vpn_ab12',
      status: 'active',
      createdBy: 'admin',
      createdAt: now,
    })
    expect(Object.keys(document)).toEqual([
      '_id',
      'label',
      'kind',
      'tokenHash',
      'prefix',
      'status',
      'createdBy',
      'createdAt',
    ])
  })

  it('stores the cap when there is one', async () => {
    await insertInvite({
      label: 'Team A',
      kind: 'global',
      tokenHash: 'hash',
      prefix: 'vpn_ab12',
      createdBy: 'admin',
      maxLeases: 4,
    })

    expect(invites.insertOne.mock.calls[0][0]).toMatchObject({ maxLeases: 4 })
  })
})

describe('setInviteMaxLeases', () => {
  it('sets the cap', async () => {
    invites.updateOne.mockResolvedValue({ matchedCount: 1 })

    expect(await setInviteMaxLeases(FIRST_ID.toHexString(), 4)).toBe(true)
    expect(invites.updateOne).toHaveBeenCalledWith({ _id: FIRST_ID }, { $set: { maxLeases: 4 } })
  })

  it('removes the cap for null', async () => {
    invites.updateOne.mockResolvedValue({ matchedCount: 1 })

    await setInviteMaxLeases(FIRST_ID.toHexString(), null)
    expect(invites.updateOne).toHaveBeenCalledWith({ _id: FIRST_ID }, { $unset: { maxLeases: '' } })
  })

  it('reports a missing invite and skips the database for an invalid id', async () => {
    invites.updateOne.mockResolvedValue({ matchedCount: 0 })

    expect(await setInviteMaxLeases(FIRST_ID.toHexString(), 4)).toBe(false)
    expect(await setInviteMaxLeases('not-an-id', 4)).toBe(false)
    expect(invites.updateOne).toHaveBeenCalledTimes(1)
  })
})

describe('deleteInvite', () => {
  it('deletes the invite by id', async () => {
    invites.deleteOne.mockResolvedValue({ deletedCount: 1 })

    expect(await deleteInvite(FIRST_ID.toHexString())).toBe(true)
    expect(invites.deleteOne).toHaveBeenCalledWith({ _id: FIRST_ID })
  })

  it('reports an invite that was already gone', async () => {
    invites.deleteOne.mockResolvedValue({ deletedCount: 0 })

    expect(await deleteInvite(FIRST_ID.toHexString())).toBe(false)
  })

  it('skips the database for an id no invite can have', async () => {
    expect(await deleteInvite('not-an-id')).toBe(false)
    expect(invites.deleteOne).not.toHaveBeenCalled()
  })
})
