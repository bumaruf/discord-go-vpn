import { beforeEach, describe, expect, it, vi } from 'vitest'
import { recordAudit, type AuditAction } from './audit'
import { writeProvisiond } from './provisiond'

vi.mock('./provisiond', () => ({ writeProvisiond: vi.fn() }))

beforeEach(() => {
  vi.mocked(writeProvisiond).mockReset().mockResolvedValue(true)
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

describe('recordAudit', () => {
  it.each([true, false])('sends the entry with ok %s', async (ok) => {
    await recordAudit({ actorId: 'admin', action: 'revokeInvite', target: 'inv1', ok, detail: 'x' })

    expect(writeProvisiond).toHaveBeenCalledWith('POST', '/admin/audit', {
      actor_id: 'admin',
      action: 'revoke_invite',
      target: 'inv1',
      ok,
      detail: 'x',
    })
  })

  it.each<[AuditAction, string]>([
    ['createInvite', 'create_invite'],
    ['reactivateInvite', 'reactivate_invite'],
    ['deleteInvite', 'delete_invite'],
    ['setInviteMaxLeases', 'set_invite_max_leases'],
    ['disconnect', 'disconnect'],
  ])('stores %s as %s', async (action, stored) => {
    await recordAudit({ actorId: 'admin', action, target: 'x', ok: true, detail: '' })
    expect(vi.mocked(writeProvisiond).mock.calls[0][2]).toMatchObject({ action: stored })
  })

  it('never throws', async () => {
    vi.mocked(writeProvisiond).mockRejectedValue(new Error('down'))
    await expect(
      recordAudit({ actorId: 'admin', action: 'disconnect', target: 'x', ok: true, detail: '' }),
    ).resolves.toBeUndefined()
  })
})
