import type { Device } from '@/lib/devices'
import type { LivePeer } from '@/lib/live-snapshot'

/** A device as the page lists it: what the database registered, plus its peer while it leases. */
export interface ListedDevice {
  /** The device id, or the lease id for a peer the database has no device for. */
  id: string
  /** Missing when the database never registered the machine, which then has no history. */
  deviceId?: string
  inviteId?: string
  name: string
  osUser?: string
  /** The masked address, for when the full one isn't known (the stream's database fallback). */
  origin?: string
  originIp?: string
  /** ISO: the last handshake while the machine is leasing, its last lease otherwise. */
  lastSeenAt?: string
  firstSeenAt?: string
  accessCount?: number
  /** Present only while the machine holds a lease; everything live comes from it. */
  peer?: LivePeer
}

/**
 * Joins the registered devices with the peers on the stream, so the list shows who is
 * disconnected next to who is online. A peer whose device the database doesn't have is kept
 * anyway: a machine on the tunnel is never left off the page.
 */
export function listedDevices(devices: Device[], peers: LivePeer[]): ListedDevice[] {
  const peerByDevice = new Map<string, LivePeer>()
  for (const peer of peers) {
    if (!peer.device_id) continue
    const current = peerByDevice.get(peer.device_id)
    // One machine can hold two leases for a moment, while the one it dropped waits for the reaper.
    if (!current || livelier(peer, current)) peerByDevice.set(peer.device_id, peer)
  }
  const registered = new Set(devices.map((device) => device.id))

  const listed = devices.map((device) => {
    const peer = peerByDevice.get(device.id)
    return {
      id: device.id,
      deviceId: device.id,
      inviteId: device.inviteId,
      name: device.name,
      osUser: device.osUser,
      origin: peer?.origin,
      originIp: peer?.origin_ip || device.lastIp,
      lastSeenAt: peer?.last_handshake ?? device.lastSeenAt,
      firstSeenAt: device.firstSeenAt,
      accessCount: device.accessCount,
      peer,
    }
  })

  const unregistered = peers
    .filter((peer) => !peer.device_id || !registered.has(peer.device_id))
    .map((peer) => ({
      id: peer.id,
      deviceId: peer.device_id,
      inviteId: peer.invite_id,
      name: peer.label,
      osUser: peer.os_user,
      origin: peer.origin,
      originIp: peer.origin_ip,
      lastSeenAt: peer.last_handshake,
      peer,
    }))

  return [...listed, ...unregistered]
}

/** Of a machine's leases, the one on the tunnel, and then the one that handshook last. */
function livelier(peer: LivePeer, than: LivePeer): boolean {
  if (peer.online !== than.online) return peer.online
  return (peer.last_handshake ?? '') > (than.last_handshake ?? '')
}
