import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { UsageSummary } from '@/lib/usage'
import { GET } from './route'

const auth = vi.hoisted(() => vi.fn())
const findDeviceUsage = vi.hoisted(() => vi.fn())
vi.mock('@/auth', () => ({ auth }))
vi.mock('@/lib/usage', () => ({ findDeviceUsage }))

const usage: UsageSummary = {
  days: [{ day: '2026-09-12', rxBytes: 100, txBytes: 10 }],
  totalRx: 100,
  totalTx: 10,
}

const request = (query = '') => new Request(`https://panel.example.com/api/usage${query}`)

beforeEach(() => {
  findDeviceUsage.mockResolvedValue(usage)
})

afterEach(() => {
  auth.mockReset()
  findDeviceUsage.mockReset()
})

describe('GET /api/usage', () => {
  it('answers 401 without a session', async () => {
    auth.mockResolvedValue(null)

    const response = await GET(request('?device=d1'))

    expect(response.status).toBe(401)
    expect(await response.json()).toEqual({ error: 'não autenticado' })
  })

  it('answers 403 to a session without the admin role', async () => {
    auth.mockResolvedValue({ user: { id: 'someone', role: 'user' } })

    const response = await GET(request('?device=d1'))

    expect(response.status).toBe(403)
    expect(await response.json()).toEqual({ error: 'sem permissão' })
  })

  describe('with an admin session', () => {
    beforeEach(() => {
      auth.mockResolvedValue({ user: { id: 'admin', role: 'admin' } })
    })

    it('answers 400 without a device', async () => {
      const response = await GET(request())

      expect(response.status).toBe(400)
      expect(await response.json()).toEqual({ error: 'informe o dispositivo' })
    })

    it('reads the device from ?device=', async () => {
      await GET(request('?device=d1'))

      expect(findDeviceUsage).toHaveBeenCalledWith('d1')
    })

    it('ignores any other parameter', async () => {
      const response = await GET(request('?dispositivo=d1'))

      expect(response.status).toBe(400)
      expect(findDeviceUsage).not.toHaveBeenCalled()
    })

    it('sends the usage', async () => {
      const response = await GET(request('?device=d1'))

      expect(response.status).toBe(200)
      expect(response.headers.get('Cache-Control')).toBe('no-store')
      expect(await response.json()).toEqual(usage)
    })

    it('answers 503 when the database does not answer', async () => {
      findDeviceUsage.mockResolvedValue(null)

      const response = await GET(request('?device=d1'))

      expect(response.status).toBe(503)
      expect(await response.json()).toEqual({ error: 'banco indisponível' })
    })
  })
})
