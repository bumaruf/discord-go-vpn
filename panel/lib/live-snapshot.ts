// What provisiond streams: must match the json tags of `peerMeasurement` and `Snapshot` in
// internal/provisiond/metrics/collector.go.

export interface LivePeer {
  id: string
  /** Empty for the legacy global token, which has no invite or device. */
  invite_id?: string
  device_id?: string
  label: string
  os_user?: string
  key: string
  ip: string
  /** Masked by provisiond; `origin_ip` is the full address, missing before the first handshake. */
  origin: string
  origin_ip?: string
  down_bps: number
  up_bps: number
  rx_bytes: number
  tx_bytes: number
  last_handshake?: string
  online: boolean
}

export interface LiveSnapshot {
  generated_at: string
  down_bps: number
  up_bps: number
  online: number
  active: number
  limit: number
  peers: LivePeer[]
}

/** Where the data on screen came from; the header shows it. */
export type LiveSource = 'live' | 'offline' | 'connecting'

export const EMPTY_SNAPSHOT: LiveSnapshot = {
  generated_at: '',
  down_bps: 0,
  up_bps: 0,
  online: 0,
  active: 0,
  limit: 0,
  peers: [],
}
