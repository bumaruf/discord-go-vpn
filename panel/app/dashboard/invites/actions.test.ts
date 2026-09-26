import { revalidatePath } from 'next/cache'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createInvite,
  deleteInvite,
  reactivateInvite,
  revokeInvite,
  setInviteMaxLeases,
} from './actions'

const requireAdmin = vi.hoisted(() => vi.fn())
const insertInvite = vi.hoisted(() => vi.fn())
const deleteInviteRecord = vi.hoisted(() => vi.fn())
const setInviteMaxLeasesRecord = vi.hoisted(() => vi.fn())
const revokeInviteOnServer = vi.hoisted(() => vi.fn())
const reactivateInviteOnServer = vi.hoisted(() => vi.fn())
const recordAudit = vi.hoisted(() => vi.fn())
vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }))
vi.mock('@/lib/require-admin', () => ({ requireAdmin }))
vi.mock('@/lib/invites', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/invites')>()),
  insertInvite,
  deleteInvite: deleteInviteRecord,
  setInviteMaxLeases: setInviteMaxLeasesRecord,
}))
vi.mock('@/lib/provisiond', () => ({ revokeInviteOnServer, reactivateInviteOnServer }))
vi.mock('@/lib/audit', () => ({ recordAudit }))

function inviteForm(kind: string | null = 'global') {
  const form = new FormData()
  form.set('rotulo', 'turma A')
  if (kind !== null) form.set('kind', kind)
  return form
}

function decode(token: string): unknown {
  return JSON.parse(Buffer.from(token.slice('dgv1_'.length), 'base64url').toString('utf8'))
}

beforeEach(() => {
  requireAdmin.mockResolvedValue({ ok: true, user: { id: 'admin', role: 'admin', name: 'Admin' } })
  revokeInviteOnServer.mockResolvedValue({ ok: true, message: 'ok', affected: 2 })
  reactivateInviteOnServer.mockResolvedValue({ ok: true, message: 'ok', affected: 0 })
  deleteInviteRecord.mockResolvedValue(true)
  setInviteMaxLeasesRecord.mockResolvedValue(true)
  vi.stubEnv('PROVISIOND_LEASE_URL', 'https://203.0.113.10:51821')
  vi.stubEnv('PROVISIOND_FINGERPRINT', 'fingerprint=')
})

afterEach(() => {
  vi.unstubAllEnvs()
  vi.clearAllMocks()
})

describe('createInvite', () => {
  it('stores the invite with its kind and returns the connection token', async () => {
    const result = await createInvite(inviteForm())

    expect(result).toMatchObject({ ok: true, message: 'Convite criado' })
    expect(insertInvite).toHaveBeenCalledWith(
      expect.objectContaining({ label: 'turma A', kind: 'global', createdBy: 'admin' }),
    )

    const token = (result as { token: string }).token
    expect(decode(token)).toEqual({
      server: 'https://203.0.113.10:51821',
      fingerprint: 'fingerprint=',
      invite: expect.stringMatching(/^vpn_/),
    })
    expect(insertInvite).toHaveBeenCalledWith(expect.objectContaining({ maxLeases: null }))
  })

  // Refreshing here would move the button out of the empty state and unmount the dialog showing
  // the token, which exists nowhere else.
  it('leaves the list to be refreshed when the dialog closes', async () => {
    await createInvite(inviteForm())

    expect(revalidatePath).not.toHaveBeenCalled()
  })

  it('stores the lease cap typed in the form', async () => {
    const form = inviteForm()
    form.set('maxLeases', '4')

    expect(await createInvite(form)).toMatchObject({ ok: true })
    expect(insertInvite).toHaveBeenCalledWith(expect.objectContaining({ maxLeases: 4 }))
  })

  it('refuses an invalid lease cap', async () => {
    const form = inviteForm()
    form.set('maxLeases', '0')

    expect(await createInvite(form)).toMatchObject({
      ok: false,
      error: expect.stringContaining('limite de conexões'),
    })
    expect(insertInvite).not.toHaveBeenCalled()
  })

  it.each([null, 'shared'])('refuses kind %j', async (kind) => {
    const result = await createInvite(inviteForm(kind))

    expect(result).toEqual({ ok: false, error: 'Escolha se o convite é individual ou global.' })
    expect(insertInvite).not.toHaveBeenCalled()
  })

  it.each(['PROVISIOND_LEASE_URL', 'PROVISIOND_FINGERPRINT'])(
    'refuses and stores nothing without %s',
    async (name) => {
      vi.stubEnv(name, '')

      const result = await createInvite(inviteForm())

      expect(result).toMatchObject({ ok: false, error: expect.stringContaining(name) })
      expect(insertInvite).not.toHaveBeenCalled()
      expect(recordAudit).not.toHaveBeenCalled()
    },
  )

  it('answers "Sem permissão." to a caller who is not the admin', async () => {
    requireAdmin.mockResolvedValue({ ok: false, status: 401 })

    expect(await createInvite(inviteForm())).toEqual({ ok: false, error: 'Sem permissão.' })
    expect(insertInvite).not.toHaveBeenCalled()
  })
})

describe('revokeInvite and reactivateInvite', () => {
  it('revokes an invite on provisiond', async () => {
    const result = await revokeInvite('invite-1')

    expect(result).toEqual({ ok: true, message: 'Convite revogado — 2 acessos derrubados' })
    expect(revokeInviteOnServer).toHaveBeenCalledWith('invite-1')
  })

  it('reactivates an invite on provisiond', async () => {
    expect(await reactivateInvite('invite-1')).toEqual({ ok: true, message: 'Convite reativado' })
    expect(reactivateInviteOnServer).toHaveBeenCalledWith('invite-1')
  })

  it('answers "Sem permissão." to a caller who is not the admin', async () => {
    requireAdmin.mockResolvedValue({ ok: false, status: 401 })

    expect(await revokeInvite('invite-1')).toEqual({ ok: false, error: 'Sem permissão.' })
    expect(revokeInviteOnServer).not.toHaveBeenCalled()
  })
})

describe('setInviteMaxLeases', () => {
  it('stores the new cap and records it', async () => {
    const result = await setInviteMaxLeases('invite-1', '3')

    expect(result).toMatchObject({ ok: true })
    expect(setInviteMaxLeasesRecord).toHaveBeenCalledWith('invite-1', 3)
    expect(recordAudit).toHaveBeenCalledWith(
      expect.objectContaining({ action: 'setInviteMaxLeases', target: 'invite-1', ok: true }),
    )
  })

  it('removes the cap for an empty value', async () => {
    await setInviteMaxLeases('invite-1', '')

    expect(setInviteMaxLeasesRecord).toHaveBeenCalledWith('invite-1', null)
  })

  it('refuses an invalid cap without touching the database', async () => {
    expect(await setInviteMaxLeases('invite-1', '500')).toMatchObject({ ok: false })
    expect(setInviteMaxLeasesRecord).not.toHaveBeenCalled()
  })

  it('reports an invite that no longer exists', async () => {
    setInviteMaxLeasesRecord.mockResolvedValue(false)

    expect(await setInviteMaxLeases('invite-1', '3')).toEqual({
      ok: false,
      error: 'Esse convite não existe mais.',
    })
  })

  it('answers "Sem permissão." to a caller who is not the admin', async () => {
    requireAdmin.mockResolvedValue({ ok: false, status: 401 })

    expect(await setInviteMaxLeases('invite-1', '3')).toEqual({
      ok: false,
      error: 'Sem permissão.',
    })
    expect(setInviteMaxLeasesRecord).not.toHaveBeenCalled()
  })
})

describe('deleteInvite', () => {
  it('revokes on provisiond before deleting the record', async () => {
    const result = await deleteInvite('invite-1')

    expect(result).toEqual({ ok: true, message: 'Convite excluído — 2 acessos derrubados' })
    expect(revokeInviteOnServer).toHaveBeenCalledWith('invite-1')
    expect(deleteInviteRecord).toHaveBeenCalledWith('invite-1')
    expect(revokeInviteOnServer.mock.invocationCallOrder[0]).toBeLessThan(
      deleteInviteRecord.mock.invocationCallOrder[0],
    )
    expect(recordAudit).toHaveBeenCalledWith(
      expect.objectContaining({ action: 'deleteInvite', target: 'invite-1', ok: true }),
    )
  })

  it('keeps the record when provisiond could not revoke it', async () => {
    revokeInviteOnServer.mockResolvedValue({
      ok: false,
      error: 'O servidor não respondeu a tempo.',
    })

    expect(await deleteInvite('invite-1')).toEqual({
      ok: false,
      error: 'O servidor não respondeu a tempo.',
    })
    expect(deleteInviteRecord).not.toHaveBeenCalled()
    expect(recordAudit).toHaveBeenCalledWith(expect.objectContaining({ ok: false }))
  })

  it('reports an invite that was already gone', async () => {
    deleteInviteRecord.mockResolvedValue(false)

    expect(await deleteInvite('invite-1')).toEqual({
      ok: false,
      error: 'Esse convite já tinha sido excluído.',
    })
  })

  it('answers "Sem permissão." to a caller who is not the admin', async () => {
    requireAdmin.mockResolvedValue({ ok: false, status: 401 })

    expect(await deleteInvite('invite-1')).toEqual({ ok: false, error: 'Sem permissão.' })
    expect(revokeInviteOnServer).not.toHaveBeenCalled()
  })
})
