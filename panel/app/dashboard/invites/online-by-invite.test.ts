import { describe, expect, it } from 'vitest'
import type { LivePeer } from '@/lib/live-snapshot'
import { countOnlineByInvite } from './online-by-invite'

function peer(id: string, inviteId: string | undefined, online: boolean): LivePeer {
  return {
    id,
    invite_id: inviteId,
    label: id,
    key: `key-${id}`,
    ip: '10.9.0.2',
    origin: '',
    down_bps: 0,
    up_bps: 0,
    rx_bytes: 0,
    tx_bytes: 0,
    online,
  }
}

describe('countOnlineByInvite', () => {
  it('counts the online peers of each invite', () => {
    const peers = [peer('a', 'one', true), peer('b', 'one', true), peer('c', 'two', true)]

    expect(countOnlineByInvite(peers)).toEqual(
      new Map([
        ['one', 2],
        ['two', 1],
      ]),
    )
  })

  it('skips offline peers and legacy token peers', () => {
    const peers = [peer('a', 'one', false), peer('b', undefined, true), peer('c', '', true)]

    expect(countOnlineByInvite(peers)).toEqual(new Map())
  })
})
