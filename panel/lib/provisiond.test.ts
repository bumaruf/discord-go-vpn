import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import actionResponse from '../../testdata/contracts/action-response.json'
import {
  disconnectLease,
  ProvisiondNotConfigured,
  reactivateInviteOnServer,
  readProvisiond,
  revokeInviteOnServer,
  writeProvisiond,
} from './provisiond'

const fetchMock = vi.fn()

beforeEach(() => {
  vi.stubEnv('PROVISIOND_URL', 'https://vpn.example.com')
  vi.stubEnv('VPN_PANEL_SECRET', 'panel-secret')
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllEnvs()
  vi.unstubAllGlobals()
  fetchMock.mockReset()
})

describe('disconnectLease', () => {
  it('reads the response', async () => {
    fetchMock.mockResolvedValue(Response.json(actionResponse))

    expect(await disconnectLease('lease-1')).toEqual({
      ok: true,
      message: 'Acesso derrubado',
      affected: 1,
    })
  })

  it('calls the disconnect route', async () => {
    fetchMock.mockResolvedValue(Response.json(actionResponse))

    await disconnectLease('lease-1')

    expect(String(fetchMock.mock.calls[0][0])).toBe(
      'https://vpn.example.com/admin/leases/lease-1/disconnect',
    )
  })
})

describe.each([
  ['revokeInviteOnServer', revokeInviteOnServer, 'revoke'],
  ['reactivateInviteOnServer', reactivateInviteOnServer, 'reactivate'],
])('%s', (_, call, action) => {
  it('calls the invite route', async () => {
    fetchMock.mockResolvedValue(Response.json(actionResponse))

    await call('invite-1')

    expect(String(fetchMock.mock.calls[0][0])).toBe(
      `https://vpn.example.com/admin/invites/invite-1/${action}`,
    )
  })
})

describe('readProvisiond', () => {
  it('sends the panel secret and returns the JSON', async () => {
    fetchMock.mockResolvedValue(Response.json({ days: [] }))

    expect(await readProvisiond('/admin/usage?from=2026-09-01')).toEqual({ days: [] })

    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toBe('https://vpn.example.com/admin/usage?from=2026-09-01')
    expect(init).toMatchObject({ method: 'GET', headers: { Authorization: 'Bearer panel-secret' } })
  })

  it('throws on an error status', async () => {
    fetchMock.mockResolvedValue(new Response('o banco não respondeu', { status: 503 }))
    await expect(readProvisiond('/admin/devices')).rejects.toThrow('503')
  })

  it('throws without the panel secret, before any request', async () => {
    vi.stubEnv('VPN_PANEL_SECRET', '')
    await expect(readProvisiond('/admin/devices')).rejects.toBeInstanceOf(ProvisiondNotConfigured)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('writeProvisiond', () => {
  it('sends the body as JSON', async () => {
    fetchMock.mockResolvedValue(Response.json(actionResponse))

    expect(await writeProvisiond('PUT', '/admin/invites/i/max-leases', { max_leases: 2 })).toBe(
      true,
    )

    const [, init] = fetchMock.mock.calls[0]
    expect(init).toMatchObject({
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: '{"max_leases":2}',
    })
  })

  it('returns false on 404', async () => {
    fetchMock.mockResolvedValue(new Response('esse convite não existe', { status: 404 }))
    expect(await writeProvisiond('DELETE', '/admin/invites/i')).toBe(false)
  })

  it('throws on any other error status', async () => {
    fetchMock.mockResolvedValue(new Response('', { status: 500 }))
    await expect(writeProvisiond('POST', '/admin/audit', {})).rejects.toThrow('500')
  })
})
