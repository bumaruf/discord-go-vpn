import type { ListedDevice } from './listed-devices'

/** From the database: the stream sends only the invite id. */
export type InviteLabels = Record<string, string>

export interface DeviceGroup {
  id: string
  label: string
  devices: ListedDevice[]
}

/** A missing label is an invite deleted from the database whose devices are still around. */
export function inviteLabel(inviteId: string | undefined, inviteLabels: InviteLabels): string {
  if (!inviteId) return 'Sem convite'
  return inviteLabels[inviteId] ?? 'Convite removido'
}

export function groupByInvite(devices: ListedDevice[], inviteLabels: InviteLabels): DeviceGroup[] {
  const groups = new Map<string, DeviceGroup>()

  for (const device of devices) {
    const id = device.inviteId ?? ''
    let group = groups.get(id)

    if (!group) {
      group = { id, label: inviteLabel(id, inviteLabels), devices: [] }
      groups.set(id, group)
    }
    group.devices.push(device)
  }

  // Largest group first, and devices without an invite always last.
  return [...groups.values()].sort((a, b) => {
    if (a.id === '') return 1
    if (b.id === '') return -1
    return b.devices.length - a.devices.length
  })
}
