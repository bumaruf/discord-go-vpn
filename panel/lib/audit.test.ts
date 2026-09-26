import { ObjectId } from 'mongodb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { recordAudit, type AuditAction } from './audit'
import { fakeCollection } from './fake-collection'
import { collection } from './mongodb'

vi.mock('./mongodb', () => ({ collection: vi.fn() }))

const auditLog = fakeCollection()
const NOW = new Date('2026-09-13T12:00:00Z')

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(NOW)
  vi.spyOn(console, 'error').mockImplementation(() => {})
  vi.mocked(collection).mockResolvedValue(auditLog.asCollection())
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  auditLog.insertOne.mockReset()
})

describe('recordAudit', () => {
  it.each([
    [true, 'ok'],
    [false, 'error'],
  ])('writes the entry with ok=%s as result %s', async (ok, result) => {
    await recordAudit({
      actorId: 'admin',
      action: 'revokeInvite',
      target: 'invite-1',
      ok,
      detail: 'detail',
    })

    expect(collection).toHaveBeenCalledWith('audit_log')
    expect(auditLog.insertOne).toHaveBeenCalledWith({
      _id: expect.any(ObjectId),
      actorId: 'admin',
      action: 'revoke_invite',
      target: 'invite-1',
      at: NOW,
      result,
      detail: 'detail',
    })
  })

  it.each<[AuditAction, string]>([
    ['createInvite', 'create_invite'],
    ['revokeInvite', 'revoke_invite'],
    ['reactivateInvite', 'reactivate_invite'],
    ['disconnect', 'disconnect'],
  ])('stores the %s action as %s', async (action, stored) => {
    await recordAudit({ actorId: 'admin', action, target: 'x', ok: true, detail: '' })

    expect(auditLog.insertOne.mock.calls[0][0].action).toBe(stored)
  })

  it.each([
    ['the connection fails', () => vi.mocked(collection).mockRejectedValueOnce(new Error('down'))],
    ['the insert fails', () => auditLog.insertOne.mockRejectedValueOnce(new Error('down'))],
  ])('logs instead of throwing when %s', async (_, fail) => {
    fail()

    await expect(
      recordAudit({
        actorId: 'admin',
        action: 'disconnect',
        target: 'lease-1',
        ok: true,
        detail: '',
      }),
    ).resolves.toBeUndefined()
    expect(console.error).toHaveBeenCalled()
  })
})
