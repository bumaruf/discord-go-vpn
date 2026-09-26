import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import actionResponse from '../../testdata/contracts/action-response.json'
import { disconnectLease, reactivateInviteOnServer, revokeInviteOnServer } from './provisiond'

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
