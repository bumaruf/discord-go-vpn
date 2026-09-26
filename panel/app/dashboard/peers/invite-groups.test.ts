import { describe, expect, it } from 'vitest'
import { groupByInvite } from './invite-groups'
import type { ListedDevice } from './listed-devices'

function device(id: string, inviteId?: string): ListedDevice {
  return { id, inviteId, name: id }
}

function summarize(groups: ReturnType<typeof groupByInvite>) {
  return groups.map((group) => ({
    id: group.id,
    label: group.label,
    devices: group.devices.map((d) => d.id),
  }))
}

describe('groupByInvite', () => {
  it('groups devices by invite with the invite label, largest group first', () => {
    const devices = [device('a', 'small'), device('b', 'large'), device('c', 'large')]

    const groups = groupByInvite(devices, { small: 'team B', large: 'team A' })

    expect(summarize(groups)).toEqual([
      { id: 'large', label: 'team A', devices: ['b', 'c'] },
      { id: 'small', label: 'team B', devices: ['a'] },
    ])
  })

  it('puts devices without an invite last, even when their group is the largest', () => {
    const devices = [device('a'), device('b'), device('c', 'invite')]

    const groups = groupByInvite(devices, { invite: 'personal' })

    expect(summarize(groups)).toEqual([
      { id: 'invite', label: 'personal', devices: ['c'] },
      { id: '', label: 'Sem convite', devices: ['a', 'b'] },
    ])
  })

  it('labels an invite missing from the database as removed', () => {
    const groups = groupByInvite([device('a', 'deleted')], {})

    expect(summarize(groups)).toEqual([
      { id: 'deleted', label: 'Convite removido', devices: ['a'] },
    ])
  })

  it('returns no groups without devices', () => {
    expect(groupByInvite([], {})).toEqual([])
  })
})
