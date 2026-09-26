import { describe, expect, it } from 'vitest'
import type { Device } from '@/lib/devices'
import type { LivePeer } from '@/lib/live-snapshot'
import { listedDevices } from './listed-devices'

function device(id: string, lastSeenAt: string, extra: Partial<Device> = {}): Device {
  return {
    id,
    inviteId: 'invite',
    name: `machine-${id}`,
    osUser: 'ana',
    lastIp: '198.51.100.7',
    firstSeenAt: '2026-08-01T00:00:00.000Z',
    lastSeenAt,
    accessCount: 3,
    ...extra,
  }
}

function peer(id: string, extra: Partial<LivePeer> = {}): LivePeer {
  return {
    id,
    label: `peer-${id}`,
    key: `key-${id}`,
    ip: '10.9.0.2',
    origin: '203.0.x.x',
    down_bps: 0,
    up_bps: 0,
    rx_bytes: 0,
    tx_bytes: 0,
    online: true,
    ...extra,
  }
}

describe('listedDevices', () => {
  it('carries the peer of a device that is leasing, and its live origin and handshake', () => {
    const devices = [device('d1', '2026-09-01T00:00:00.000Z')]
    const peers = [
      peer('lease-1', {
        device_id: 'd1',
        origin: '203.0.x.x',
        origin_ip: '203.0.113.1',
        last_handshake: '2026-09-20T12:00:00.000Z',
      }),
    ]

    expect(listedDevices(devices, peers)).toEqual([
      {
        id: 'd1',
        deviceId: 'd1',
        inviteId: 'invite',
        name: 'machine-d1',
        osUser: 'ana',
        origin: '203.0.x.x',
        originIp: '203.0.113.1',
        lastSeenAt: '2026-09-20T12:00:00.000Z',
        firstSeenAt: '2026-08-01T00:00:00.000Z',
        accessCount: 3,
        peer: peers[0],
      },
    ])
  })

  it('keeps a device with no peer, falling back to what the database last saw', () => {
    const [listed] = listedDevices([device('d1', '2026-09-01T00:00:00.000Z')], [])

    expect(listed.peer).toBeUndefined()
    expect(listed.originIp).toBe('198.51.100.7')
    expect(listed.lastSeenAt).toBe('2026-09-01T00:00:00.000Z')
  })

  it('keeps a peer the database has no device for, with no history to show', () => {
    const listed = listedDevices([], [peer('lease-1', { label: 'sem registro' })])

    expect(listed).toEqual([
      {
        id: 'lease-1',
        deviceId: undefined,
        inviteId: undefined,
        name: 'sem registro',
        osUser: undefined,
        origin: '203.0.x.x',
        originIp: undefined,
        lastSeenAt: undefined,
        peer: listed[0].peer,
      },
    ])
  })
})

describe('listedDevices with two leases for one machine', () => {
  it('keeps the lease that is on the tunnel', () => {
    const peers = [
      peer('dropped', { device_id: 'd1', online: false }),
      peer('current', { device_id: 'd1', online: true }),
    ]

    const [listed] = listedDevices([device('d1', '2026-09-01T00:00:00.000Z')], peers)

    expect(listed.peer?.id).toBe('current')
  })

  it('keeps the lease that handshook last when both are on the tunnel', () => {
    const peers = [
      peer('older', { device_id: 'd1', last_handshake: '2026-09-20T11:00:00.000Z' }),
      peer('newer', { device_id: 'd1', last_handshake: '2026-09-20T12:00:00.000Z' }),
    ]

    const [listed] = listedDevices([device('d1', '2026-09-01T00:00:00.000Z')], peers)

    expect(listed.peer?.id).toBe('newer')
  })
})
