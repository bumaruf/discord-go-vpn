import type { Collection } from 'mongodb'
import { vi } from 'vitest'

// Test-only stand-ins for the driver, so repositories are tested without a real Mongo.

export function fakeCursor<T>(docs: T[]) {
  const cursor = { sort: vi.fn(), limit: vi.fn(), toArray: vi.fn(async () => docs) }
  cursor.sort.mockReturnValue(cursor)
  cursor.limit.mockReturnValue(cursor)
  return cursor
}

export function fakeCollection() {
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
