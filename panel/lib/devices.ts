import 'server-only'

import { readProvisiond } from './provisiond'

/** GET /admin/devices (testdata/contracts/panel-devices.json). */
export interface DevicesResponse {
  devices: {
    id: string
    invite_id?: string
    name: string
    os_user?: string
    last_ip?: string
    first_seen_at: string
    last_seen_at: string
    access_count: number
  }[]
  invite_labels: Record<string, string>
}

/** Dates as ISO strings: the list is rendered by a client component. */
export interface Device {
  id: string
  inviteId?: string
  name: string
  osUser?: string
  lastIp?: string
  firstSeenAt: string
  lastSeenAt: string
  accessCount: number
}

export interface DeviceDirectory {
  devices: Device[]
  /** Invite labels keyed by invite id. */
  inviteLabels: Record<string, string>
}

/**
 * Every registered machine, the most recently seen first — including the ones not connected now,
 * which hold the usage history of their leases.
 */
export async function listDevices(): Promise<DeviceDirectory> {
  const response = await readProvisiond<DevicesResponse>('/admin/devices')
  return {
    devices: response.devices.map((device) => ({
      id: device.id,
      inviteId: device.invite_id,
      name: device.name,
      osUser: device.os_user,
      lastIp: device.last_ip,
      firstSeenAt: device.first_seen_at,
      lastSeenAt: device.last_seen_at,
      accessCount: device.access_count,
    })),
    inviteLabels: response.invite_labels,
  }
}
