import { ObjectId } from 'mongodb'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { collection } from './mongodb'
import { runOperation } from './mongo-operation'

vi.mock('./mongo-operation', () => ({
  connectMongo: vi.fn(async () => ({ db: () => 'db' })),
  runOperation: vi.fn(),
}))

beforeEach(() => {
  vi.mocked(runOperation).mockReset()
})

describe('collection', () => {
  it('turns a find cursor into one find with sort and limit as options', async () => {
    vi.mocked(runOperation).mockResolvedValue([{ _id: 1 }])
    const invites = await collection('invites')

    const docs = await invites
      .find({ status: 'active' }, { projection: { label: 1 } })
      .sort({ createdAt: -1 })
      .limit(5)
      .toArray()

    expect(docs).toEqual([{ _id: 1 }])
    expect(runOperation).toHaveBeenCalledWith('db', {
      collection: 'invites',
      op: 'find',
      filter: { status: 'active' },
      options: { projection: { label: 1 }, sort: { createdAt: -1 }, limit: 5 },
    })
  })

  it('passes writes through with their arguments', async () => {
    vi.mocked(runOperation).mockResolvedValue({ matchedCount: 1 })
    const id = new ObjectId()
    const invites = await collection('invites')

    const result = await invites.updateOne({ _id: id }, { $set: { status: 'revoked' } })

    expect(result).toEqual({ matchedCount: 1 })
    expect(runOperation).toHaveBeenCalledWith('db', {
      collection: 'invites',
      op: 'updateOne',
      filter: { _id: id },
      update: { $set: { status: 'revoked' } },
      options: undefined,
    })
  })
})
