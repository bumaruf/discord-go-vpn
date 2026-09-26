import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fakeCollection, fakeCursor } from './fake-collection'
import { collection } from './mongodb'
import { findLatestServerSample } from './samples'

vi.mock('./mongodb', () => ({ collection: vi.fn() }))

const samples = fakeCollection()

beforeEach(() => {
  samples.find.mockReset()
  vi.mocked(collection).mockResolvedValue(samples.asCollection())
})

describe('findLatestServerSample', () => {
  it('reads the newest server-scope sample', async () => {
    const cursor = fakeCursor([])
    samples.find.mockReturnValue(cursor)

    await findLatestServerSample()

    expect(collection).toHaveBeenCalledWith('samples')
    expect(samples.find).toHaveBeenCalledWith({ 'meta.scope': 'server' })
    expect(cursor.sort).toHaveBeenCalledWith({ ts: -1 })
    expect(cursor.limit).toHaveBeenCalledWith(1)
  })

  it('maps the stored sample', async () => {
    const at = new Date('2026-09-12T10:05:00Z')
    samples.find.mockReturnValue(
      fakeCursor([{ ts: at, meta: { scope: 'server' }, downBps: 800, upBps: 80 }]),
    )

    expect(await findLatestServerSample()).toEqual({ at, downBps: 800, upBps: 80 })
  })

  it('returns undefined when no sample is stored', async () => {
    samples.find.mockReturnValue(fakeCursor([]))

    expect(await findLatestServerSample()).toBeUndefined()
  })
})
