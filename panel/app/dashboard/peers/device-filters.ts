import { deviceName } from './device-name'
import { inviteLabel, type InviteLabels } from './invite-groups'
import type { ListedDevice } from './listed-devices'

/** Off the tunnel for longer than this, a machine leaves the list unless `inactive` is set. */
export const INACTIVE_AFTER_DAYS = 5
const DAY_MS = 24 * 60 * 60 * 1000

export type StatusFilter = 'all' | 'online' | 'offline'
export type SortKey = 'name' | 'traffic' | 'session'
export type SortDir = 'asc' | 'desc'

/** Everything the toolbar sets. It lives in the query string, so a filtered view can be linked. */
export interface DeviceFilters {
  query: string
  status: StatusFilter
  /** Invite ids to show; `''` stands for the devices without an invite. Empty shows every invite. */
  invites: string[]
  group: boolean
  /** Also shows the machines that haven't connected for `INACTIVE_AFTER_DAYS`. */
  inactive: boolean
  /** Missing is the default order: whoever is on the tunnel first, then by name. */
  sort?: SortKey
  dir: SortDir
}

/** `''` can't travel in a list separated by commas. */
const NO_INVITE_PARAM = 'none'

/** The query string keys this page owns, besides `device`. */
const FILTER_PARAMS = ['q', 'status', 'invite', 'group', 'inactive', 'sort', 'dir']

const STATUSES: StatusFilter[] = ['all', 'online', 'offline']
const SORT_KEYS: SortKey[] = ['name', 'traffic', 'session']

export function parseFilters(params: URLSearchParams): DeviceFilters {
  const status = params.get('status') as StatusFilter
  const sort = params.get('sort') as SortKey
  const invites = params.get('invite')

  return {
    query: params.get('q') ?? '',
    status: STATUSES.includes(status) ? status : 'all',
    invites: invites ? invites.split(',').map((id) => (id === NO_INVITE_PARAM ? '' : id)) : [],
    group: params.get('group') === '1',
    inactive: params.get('inactive') === '1',
    sort: SORT_KEYS.includes(sort) ? sort : undefined,
    dir: params.get('dir') === 'desc' ? 'desc' : 'asc',
  }
}

/** Writes the filters over `params`, leaving out what is at its default and keeping other keys. */
export function filtersToParams(filters: DeviceFilters, params: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams(params)
  for (const key of FILTER_PARAMS) next.delete(key)

  if (filters.query) next.set('q', filters.query)
  if (filters.status !== 'all') next.set('status', filters.status)
  if (filters.invites.length > 0) {
    next.set('invite', filters.invites.map((id) => id || NO_INVITE_PARAM).join(','))
  }
  if (filters.group) next.set('group', '1')
  if (filters.inactive) next.set('inactive', '1')
  if (filters.sort) {
    next.set('sort', filters.sort)
    if (filters.dir === 'desc') next.set('dir', 'desc')
  }
  return next
}

/** How many filters narrow the list, for the badge on the button that holds the hidden ones. */
export function hiddenFilterCount(filters: DeviceFilters): number {
  return filters.invites.length + Number(filters.group) + Number(filters.inactive)
}

/** A machine holding a lease is active however long ago it leased. */
export function isInactive(device: ListedDevice, now = Date.now()): boolean {
  if (device.peer) return false
  const seen = device.lastSeenAt ? Date.parse(device.lastSeenAt) : NaN
  return !(now - seen <= INACTIVE_AFTER_DAYS * DAY_MS)
}

export function filterDevices(
  devices: ListedDevice[],
  filters: DeviceFilters,
  inviteLabels: InviteLabels,
  now = Date.now(),
): ListedDevice[] {
  const terms = normalize(filters.query).split(/\s+/).filter(Boolean)
  const invites = new Set(filters.invites)

  return devices.filter((device) => {
    if (!filters.inactive && isInactive(device, now)) return false
    const online = device.peer?.online ?? false
    if (filters.status === 'online' && !online) return false
    if (filters.status === 'offline' && online) return false
    if (invites.size > 0 && !invites.has(device.inviteId ?? '')) return false
    if (terms.length === 0) return true

    const haystack = normalize(
      [
        deviceName(device),
        device.peer?.ip,
        device.originIp,
        device.origin,
        inviteLabel(device.inviteId, inviteLabels),
      ].join(' '),
    )
    return terms.every((term) => haystack.includes(term))
  })
}

export function sortDevices(devices: ListedDevice[], filters: DeviceFilters): ListedDevice[] {
  const { sort } = filters
  if (!sort) return [...devices].sort(byOnlineThenName)

  const sign = filters.dir === 'desc' ? -1 : 1
  return [...devices].sort((a, b) => {
    if (sort === 'name') return sign * byName(a, b)
    const left = sortValue(a, sort)
    const right = sortValue(b, sort)
    // A machine off the tunnel has nothing to compare, so it stays at the bottom either way.
    if (left === undefined || right === undefined) {
      return Number(left === undefined) - Number(right === undefined) || byName(a, b)
    }
    return sign * (left - right) || byName(a, b)
  })
}

function sortValue(device: ListedDevice, sort: 'traffic' | 'session'): number | undefined {
  const peer = device.peer
  if (!peer) return undefined
  return sort === 'traffic' ? peer.down_bps + peer.up_bps : peer.rx_bytes + peer.tx_bytes
}

/** Sorting by name keeps rows still while the stream ticks, which sorting by traffic wouldn't. */
function byOnlineThenName(a: ListedDevice, b: ListedDevice): number {
  const online = Number(b.peer?.online ?? false) - Number(a.peer?.online ?? false)
  return online || byName(a, b)
}

function byName(a: ListedDevice, b: ListedDevice): number {
  return deviceName(a).localeCompare(deviceName(b), 'pt-BR', { sensitivity: 'base' })
}

/** Lowercase and without accents, so "joao" finds "João". */
function normalize(text: string): string {
  return text
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase()
}
