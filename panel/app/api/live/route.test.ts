import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GET } from './route'

const auth = vi.hoisted(() => vi.fn())
vi.mock('@/auth', () => ({ auth }))
vi.mock('@/lib/mongodb', () => ({
  collection: vi.fn(async () => {
    throw new Error('atlas down')
  }),
}))

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  auth.mockReset()
  vi.restoreAllMocks()
})

describe('GET /api/live', () => {
  it('answers 401 without a session', async () => {
    auth.mockResolvedValue(null)

    const response = await GET()

    expect(response.status).toBe(401)
    expect(await response.json()).toEqual({ error: 'não autenticado' })
  })

  it('answers 403 to a session without the admin role', async () => {
    auth.mockResolvedValue({ user: { id: 'someone', role: 'user' } })

    const response = await GET()

    expect(response.status).toBe(403)
    expect(await response.json()).toEqual({ error: 'sem permissão' })
  })

  it('lets an admin session reach the database', async () => {
    auth.mockResolvedValue({ user: { id: 'admin', role: 'admin' } })

    const response = await GET()

    expect(response.status).toBe(503)
  })
})
