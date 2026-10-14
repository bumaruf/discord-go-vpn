import { beforeEach, describe, expect, it, vi } from 'vitest'
import invitesFixture from '../../testdata/contracts/panel-invites.json'
import { deleteInvite, insertInvite, listRecentInvites, setInviteMaxLeases } from './invites'
import { readProvisiond, writeProvisiond } from './provisiond'

vi.mock('./provisiond', () => ({ readProvisiond: vi.fn(), writeProvisiond: vi.fn() }))

beforeEach(() => {
  vi.mocked(readProvisiond).mockReset()
  vi.mocked(writeProvisiond).mockReset().mockResolvedValue(true)
})

describe('listRecentInvites', () => {
  it('maps each invite provisiond lists', async () => {
    vi.mocked(readProvisiond).mockResolvedValue(invitesFixture)

    expect(await listRecentInvites()).toEqual([
      {
        id: '65f1a2b3c4d5e6f7a8b9c0d1',
        label: 'Família',
        kind: 'global',
        prefix: 'vpn_ab12',
        active: true,
        createdAt: new Date('2026-09-12T12:00:00Z'),
        maxLeases: 5,
        deviceCount: 3,
      },
      {
        id: '65f1a2b3c4d5e6f7a8b9c0d2',
        label: 'notebook da ana',
        kind: 'individual',
        prefix: 'vpn_cd34',
        active: false,
        createdAt: new Date('2026-09-10T08:30:00Z'),
        maxLeases: null,
        deviceCount: 0,
      },
    ])
    expect(readProvisiond).toHaveBeenCalledWith('/admin/invites')
  })

  it('reads an unknown kind as individual', async () => {
    const [first] = invitesFixture.invites
    vi.mocked(readProvisiond).mockResolvedValue({ invites: [{ ...first, kind: 'family' }] })

    const [invite] = await listRecentInvites()
    expect(invite.kind).toBe('individual')
  })
})

describe('insertInvite', () => {
  const invite = {
    label: 'Família',
    kind: 'global' as const,
    tokenHash: 'hash',
    prefix: 'vpn_ab12',
    createdBy: 'admin',
  }

  it('sends the invite without a cap when it has none', async () => {
    await insertInvite({ ...invite, maxLeases: null })

    expect(writeProvisiond).toHaveBeenCalledWith('POST', '/admin/invites', {
      label: 'Família',
      kind: 'global',
      token_hash: 'hash',
      prefix: 'vpn_ab12',
      created_by: 'admin',
    })
  })

  it('sends the cap when there is one', async () => {
    await insertInvite({ ...invite, maxLeases: 5 })
    expect(vi.mocked(writeProvisiond).mock.calls[0][2]).toMatchObject({ max_leases: 5 })
  })

  it('throws when the route is missing', async () => {
    vi.mocked(writeProvisiond).mockResolvedValue(false)
    await expect(insertInvite({ ...invite, maxLeases: null })).rejects.toThrow()
  })
})

describe('setInviteMaxLeases', () => {
  it.each([5, null])('sends %j as the cap', async (cap) => {
    expect(await setInviteMaxLeases('65f1a2b3c4d5e6f7a8b9c0d1', cap)).toBe(true)
    expect(writeProvisiond).toHaveBeenCalledWith(
      'PUT',
      '/admin/invites/65f1a2b3c4d5e6f7a8b9c0d1/max-leases',
      { max_leases: cap },
    )
  })

  it('reports a missing invite', async () => {
    vi.mocked(writeProvisiond).mockResolvedValue(false)
    expect(await setInviteMaxLeases('gone', 5)).toBe(false)
  })
})

describe('deleteInvite', () => {
  it('deletes the invite by id, escaped', async () => {
    expect(await deleteInvite('a/b')).toBe(true)
    expect(writeProvisiond).toHaveBeenCalledWith('DELETE', '/admin/invites/a%2Fb')
  })

  it('reports an invite that was already gone', async () => {
    vi.mocked(writeProvisiond).mockResolvedValue(false)
    expect(await deleteInvite('65f1a2b3c4d5e6f7a8b9c0d1')).toBe(false)
  })
})
