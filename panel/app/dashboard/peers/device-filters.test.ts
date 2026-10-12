import { describe, expect, it } from 'vitest'
import type { LivePeer } from '@/lib/live-snapshot'
import {
  filterDevices,
  filtersToParams,
  hiddenFilterCount,
  isInactive,
  parseFilters,
  sortDevices,
  type DeviceFilters,
} from './device-filters'
import type { ListedDevice } from './listed-devices'

const DEFAULTS: DeviceFilters = {
  query: '',
  status: 'all',
  invites: [],
  group: false,
  inactive: false,
  dir: 'asc',
}

const NOW = Date.parse('2026-10-12T12:00:00Z')
const daysAgo = (days: number) => new Date(NOW - days * 24 * 60 * 60 * 1000).toISOString()

function device(id: string, extra: Partial<ListedDevice> = {}): ListedDevice {
  return { id, name: id, lastSeenAt: daysAgo(1), ...extra }
}

function peer(extra: Partial<LivePeer> = {}): LivePeer {
  return {
    id: 'lease',
    label: 'lease',
    key: 'key',
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

const ids = (devices: ListedDevice[]) => devices.map((d) => d.id)

describe('parseFilters and filtersToParams', () => {
  it('reads the defaults from an empty query string', () => {
    expect(parseFilters(new URLSearchParams())).toEqual(DEFAULTS)
  })

  it('round-trips every filter, with the invite-less group as "none"', () => {
    const filters: DeviceFilters = {
      query: 'joão',
      status: 'offline',
      invites: ['inv-1', ''],
      group: true,
      inactive: true,
      sort: 'traffic',
      dir: 'desc',
    }
    const params = filtersToParams(filters, new URLSearchParams())

    expect(params.get('invite')).toBe('inv-1,none')
    expect(parseFilters(params)).toEqual(filters)
  })

  it('leaves defaults out and keeps the keys it does not own', () => {
    const params = filtersToParams(DEFAULTS, new URLSearchParams('q=old&status=online&x=1'))
    expect(params.toString()).toBe('x=1')
  })

  it('ignores values it does not know', () => {
    const filters = parseFilters(new URLSearchParams('status=maybe&sort=rtt&dir=up'))
    expect(filters).toEqual(DEFAULTS)
  })
})

describe('hiddenFilterCount', () => {
  it('counts each invite, the grouping and the inactive machines, behind the Filters button', () => {
    expect(hiddenFilterCount({ ...DEFAULTS, query: 'x', status: 'online' })).toBe(0)
    expect(hiddenFilterCount({ ...DEFAULTS, invites: ['a', ''], group: true })).toBe(3)
    expect(hiddenFilterCount({ ...DEFAULTS, inactive: true })).toBe(1)
  })
})

describe('filterDevices', () => {
  const devices = [
    device('desktop', { osUser: 'João', inviteId: 'friends', peer: peer({ ip: '10.9.0.4' }) }),
    device('notebook', { osUser: 'ana', inviteId: 'family', originIp: '198.51.100.7' }),
    device('orphan'),
  ]
  const labels = { friends: 'Amigos', family: 'Família' }

  it('searches name, OS user, IPs and invite, ignoring case and accents', () => {
    const search = (query: string) =>
      ids(filterDevices(devices, { ...DEFAULTS, query }, labels, NOW))

    expect(search('joao')).toEqual(['desktop'])
    expect(search('10.9.0.4')).toEqual(['desktop'])
    expect(search('198.51')).toEqual(['notebook'])
    expect(search('familia')).toEqual(['notebook'])
    expect(search('sem convite')).toEqual(['orphan'])
    expect(search('ana notebook')).toEqual(['notebook'])
    expect(search('ana desktop')).toEqual([])
  })

  it('filters by status, a peer that lost its handshake counting as off the tunnel', () => {
    const withStale = [...devices, device('stale', { peer: peer({ online: false }) })]
    const status = (s: DeviceFilters['status']) =>
      ids(filterDevices(withStale, { ...DEFAULTS, status: s }, labels, NOW))

    expect(status('online')).toEqual(['desktop'])
    expect(status('offline')).toEqual(['notebook', 'orphan', 'stale'])
  })

  it('keeps only the chosen invites, with "" for the machines without one', () => {
    const filters = { ...DEFAULTS, invites: ['family', ''] }
    expect(ids(filterDevices(devices, filters, labels, NOW))).toEqual(['notebook', 'orphan'])
  })

  it('hides the machines off the tunnel for more than 5 days unless asked for', () => {
    const withOld = [
      ...devices,
      device('old', { lastSeenAt: daysAgo(6) }),
      device('leasing', { lastSeenAt: daysAgo(30), peer: peer({ online: false }) }),
    ]

    expect(ids(filterDevices(withOld, DEFAULTS, labels, NOW))).toEqual([
      'desktop',
      'notebook',
      'orphan',
      'leasing',
    ])
    expect(ids(filterDevices(withOld, { ...DEFAULTS, inactive: true }, labels, NOW))).toContain(
      'old',
    )
  })
})

describe('isInactive', () => {
  it('counts from the last time seen, and never while the machine holds a lease', () => {
    expect(isInactive(device('a', { lastSeenAt: daysAgo(5) }), NOW)).toBe(false)
    expect(isInactive(device('a', { lastSeenAt: daysAgo(5.1) }), NOW)).toBe(true)
    expect(isInactive(device('a', { lastSeenAt: undefined }), NOW)).toBe(true)
    expect(isInactive(device('a', { lastSeenAt: daysAgo(9), peer: peer() }), NOW)).toBe(false)
  })
})

describe('sortDevices', () => {
  const devices = [
    device('bravo', { peer: peer({ online: false, down_bps: 900, rx_bytes: 5 }) }),
    device('delta'),
    device('charlie', { peer: peer({ down_bps: 10, up_bps: 5, rx_bytes: 100 }) }),
    device('alpha', { peer: peer({ down_bps: 500, tx_bytes: 1 }) }),
  ]

  it('puts whoever is on the tunnel first, then sorts by name', () => {
    expect(ids(sortDevices(devices, DEFAULTS))).toEqual(['alpha', 'charlie', 'bravo', 'delta'])
  })

  it('sorts by name either way', () => {
    const sorted = sortDevices(devices, { ...DEFAULTS, sort: 'name', dir: 'desc' })
    expect(ids(sorted)).toEqual(['delta', 'charlie', 'bravo', 'alpha'])
  })

  it('sorts by traffic, keeping machines without a lease at the bottom both ways', () => {
    const desc = sortDevices(devices, { ...DEFAULTS, sort: 'traffic', dir: 'desc' })
    const asc = sortDevices(devices, { ...DEFAULTS, sort: 'traffic', dir: 'asc' })

    expect(ids(desc)).toEqual(['bravo', 'alpha', 'charlie', 'delta'])
    expect(ids(asc)).toEqual(['charlie', 'alpha', 'bravo', 'delta'])
  })

  it('sorts by what the session moved', () => {
    const sorted = sortDevices(devices, { ...DEFAULTS, sort: 'session', dir: 'desc' })
    expect(ids(sorted)).toEqual(['charlie', 'bravo', 'alpha', 'delta'])
  })
})
