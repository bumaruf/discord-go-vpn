import type { LivePeer } from './live-snapshot'
import type { Session } from './sessions'

export function peerFromSession(session: Session): LivePeer {
  return {
    id: session.leaseId,
    invite_id: session.inviteId,
    device_id: session.deviceId,
    label: session.label ?? '',
    key: session.publicKey ?? '',
    ip: session.ip ?? '',
    origin: session.originIp ?? '',
    // The database stores running totals, not an instantaneous rate: without the
    // stream there is no "now" to show, and zero is the honest answer.
    down_bps: 0,
    up_bps: 0,
    rx_bytes: session.rxBytes ?? 0,
    tx_bytes: session.txBytes ?? 0,
    last_handshake: session.lastHandshake?.toISOString(),
    online: session.online,
  }
}
