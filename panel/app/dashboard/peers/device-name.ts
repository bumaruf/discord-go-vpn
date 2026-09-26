import type { ListedDevice } from './listed-devices'

/** The hostname the client sent, followed by the OS user when there is one. */
export function deviceName(device: Pick<ListedDevice, 'name' | 'osUser'>): string {
  const name = device.name || 'sem nome'
  return device.osUser ? `${name} · ${device.osUser}` : name
}
