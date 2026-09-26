import { describe, expect, it } from 'vitest'
import { peerFromSession } from './live-fallback'
import type { Session } from './sessions'

const session: Session = {
  leaseId: 'lease-1',
  inviteId: 'invite-1',
  deviceId: 'device-1',
  label: 'desktop',
  publicKey: 'pub-key',
  ip: '10.9.0.2',
  originIp: '203.0.113.7',
  lastHandshake: new Date('2026-09-12T10:05:00Z'),
  rxBytes: 1_000,
  txBytes: 2_000,
  online: true,
}

describe('peerFromSession', () => {
  it('builds the peer from what the collector writes to sessions', () => {
    expect(peerFromSession(session)).toEqual({
      id: 'lease-1',
      invite_id: 'invite-1',
      device_id: 'device-1',
      label: 'desktop',
      key: 'pub-key',
      ip: '10.9.0.2',
      origin: '203.0.113.7',
      down_bps: 0,
      up_bps: 0,
      rx_bytes: 1_000,
      tx_bytes: 2_000,
      last_handshake: '2026-09-12T10:05:00.000Z',
      online: true,
    })
  })

  it('is offline when the session is offline', () => {
    expect(peerFromSession({ ...session, online: false }).online).toBe(false)
  })

  it('fills fields missing from the document with empty values', () => {
    const sparse = { leaseId: 'lease-2', online: false } as Session

    expect(peerFromSession(sparse)).toMatchObject({
      label: '',
      key: '',
      ip: '',
      origin: '',
      rx_bytes: 0,
      tx_bytes: 0,
      last_handshake: undefined,
    })
  })
})
