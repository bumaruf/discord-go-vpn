import { afterEach, describe, expect, it, vi } from 'vitest'
import { deniedResponse, requireAdmin } from './require-admin'

const auth = vi.hoisted(() => vi.fn())
vi.mock('@/auth', () => ({ auth }))

afterEach(() => {
  auth.mockReset()
})

describe('requireAdmin', () => {
  it.each([
    ['no session', null],
    ['a session without a user', {}],
  ])('denies with 401 for %s', async (_, session) => {
    auth.mockResolvedValue(session)
    expect(await requireAdmin()).toEqual({ ok: false, status: 401 })
  })

  it('denies with 403 for a session without the admin role', async () => {
    auth.mockResolvedValue({ user: { id: 'someone', role: 'user' } })
    expect(await requireAdmin()).toEqual({ ok: false, status: 403 })
  })

  it('returns the user of an admin session', async () => {
    const user = { id: 'admin', role: 'admin', name: 'Admin' }
    auth.mockResolvedValue({ user })
    expect(await requireAdmin()).toEqual({ ok: true, user })
  })
})

describe('deniedResponse', () => {
  it.each([
    [401, 'não autenticado'],
    [403, 'sem permissão'],
  ] as const)('answers %i with its error', async (status, error) => {
    const response = deniedResponse(status)
    expect(response.status).toBe(status)
    expect(await response.json()).toEqual({ error })
  })
})
