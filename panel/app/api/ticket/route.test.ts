import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GET } from './route'

const auth = vi.hoisted(() => vi.fn())
vi.mock('@/auth', () => ({ auth }))

beforeEach(() => {
  vi.stubEnv('PROVISIOND_URL', 'https://vpn.example.com/')
  vi.stubEnv('VPN_LIVE_SECRET', 'test-live-secret')
})

afterEach(() => {
  auth.mockReset()
  vi.unstubAllEnvs()
})

describe('GET /api/ticket', () => {
  it('answers 401 without a session', async () => {
    auth.mockResolvedValue(null)

    const response = await GET()

    expect(response.status).toBe(401)
    expect(await response.json()).not.toHaveProperty('ticket')
  })

  it('answers 403 to a session without the admin role', async () => {
    auth.mockResolvedValue({ user: { id: 'someone', role: 'user' } })

    const response = await GET()

    expect(response.status).toBe(403)
    expect(await response.json()).not.toHaveProperty('ticket')
  })

  it('issues a ticket and the stream URL to an admin session', async () => {
    auth.mockResolvedValue({ user: { id: 'admin', role: 'admin' } })

    const response = await GET()
    const body = await response.json()

    expect(response.status).toBe(200)
    expect(body).toEqual({
      ticket: expect.stringMatching(/^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/),
      url: 'https://vpn.example.com/admin/live',
    })
  })

  it('answers 503 without PROVISIOND_URL', async () => {
    auth.mockResolvedValue({ user: { id: 'admin', role: 'admin' } })
    vi.stubEnv('PROVISIOND_URL', undefined)

    const response = await GET()

    expect(response.status).toBe(503)
    expect(await response.json()).toEqual({ error: 'PROVISIOND_URL não configurada' })
  })
})
