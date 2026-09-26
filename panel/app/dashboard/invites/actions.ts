'use server'

import { revalidatePath } from 'next/cache'
import { resultMessage, type ActionResult } from '@/lib/action-result'
import { recordAudit } from '@/lib/audit'
import { connectionTokenFor } from '@/lib/connection-token'
import {
  deleteInvite as deleteInviteRecord,
  insertInvite,
  isInviteKind,
  setInviteMaxLeases as setInviteMaxLeasesRecord,
} from '@/lib/invites'
import { MAX_LEASES_LIMIT, parseMaxLeases } from '@/lib/max-leases'
import { generateInvite } from '@/lib/token'
import { reactivateInviteOnServer, revokeInviteOnServer } from '@/lib/provisiond'
import { requireAdmin } from '@/lib/require-admin'

/** The only time the plaintext token exists: the database keeps just the invite's hash. */
export async function createInvite(formData: FormData): Promise<ActionResult<{ token: string }>> {
  const admin = await requireAdmin()
  if (!admin.ok) return { ok: false, error: 'Sem permissão.' }

  const label = String(formData.get('rotulo') ?? '').trim() || 'sem nome'
  if (label.length > 60) {
    return { ok: false, error: 'O nome do convite passou de 60 caracteres.' }
  }

  const kind = formData.get('kind')
  if (!isInviteKind(kind)) {
    return { ok: false, error: 'Escolha se o convite é individual ou global.' }
  }

  const maxLeases = parseMaxLeases(formData.get('maxLeases'))
  if (maxLeases === undefined) return { ok: false, error: INVALID_MAX_LEASES }

  const invite = generateInvite()
  // Checked before inserting: an invite without a token to hand out would be unusable.
  const token = connectionTokenFor(invite.code)
  if (!token) {
    return {
      ok: false,
      error:
        'Faltam PROVISIOND_LEASE_URL e PROVISIOND_FINGERPRINT no painel: sem eles não há como montar o token de conexão.',
    }
  }

  try {
    await insertInvite({
      label,
      kind,
      tokenHash: invite.hash,
      prefix: invite.prefix,
      createdBy: admin.user.id,
      maxLeases,
    })

    await recordAudit({
      actorId: admin.user.id,
      action: 'createInvite',
      target: label,
      ok: true,
      detail: `${invite.prefix} ${kind} ${maxLeasesDetail(maxLeases)}`,
    })

    // The list is refreshed only when the dialog closes: refreshing here moves the button that
    // opens it out of the empty state, which unmounts the dialog and takes the token with it.
    return { ok: true, message: 'Convite criado', token }
  } catch (error) {
    console.error('failed to create invite', error)
    return { ok: false, error: 'Não consegui criar o convite.' }
  }
}

/**
 * provisiond updates the invite, clears its cache and drops the devices online, so a failed call
 * never leaves an invite marked revoked while it still carries traffic.
 */
export async function revokeInvite(inviteId: string): Promise<ActionResult> {
  const admin = await requireAdmin()
  if (!admin.ok) return { ok: false, error: 'Sem permissão.' }

  const result = await revokeInviteOnServer(inviteId)
  await recordAudit({
    actorId: admin.user.id,
    action: 'revokeInvite',
    target: inviteId,
    ok: result.ok,
    detail: resultMessage(result),
  })

  if (!result.ok) return { ok: false, error: result.error }

  revalidatePath('/dashboard/invites')
  revalidatePath('/dashboard/peers')

  return { ok: true, message: `Convite revogado — ${droppedAccesses(result.affected)}` }
}

export async function reactivateInvite(inviteId: string): Promise<ActionResult> {
  const admin = await requireAdmin()
  if (!admin.ok) return { ok: false, error: 'Sem permissão.' }

  const result = await reactivateInviteOnServer(inviteId)
  await recordAudit({
    actorId: admin.user.id,
    action: 'reactivateInvite',
    target: inviteId,
    ok: result.ok,
    detail: resultMessage(result),
  })

  if (!result.ok) return { ok: false, error: result.error }

  revalidatePath('/dashboard/invites')
  return { ok: true, message: 'Convite reativado' }
}

/**
 * Revoking on provisiond first drops the devices online and clears the cached token, so a deleted
 * invite never keeps carrying traffic. Its devices, sessions and usage stay in the database.
 */
export async function deleteInvite(inviteId: string): Promise<ActionResult> {
  const admin = await requireAdmin()
  if (!admin.ok) return { ok: false, error: 'Sem permissão.' }

  const revoked = await revokeInviteOnServer(inviteId)
  if (!revoked.ok) {
    await recordAudit({
      actorId: admin.user.id,
      action: 'deleteInvite',
      target: inviteId,
      ok: false,
      detail: resultMessage(revoked),
    })
    return { ok: false, error: revoked.error }
  }

  let deleted: boolean
  try {
    deleted = await deleteInviteRecord(inviteId)
  } catch (error) {
    console.error('failed to delete invite', error)
    return {
      ok: false,
      error: 'O convite foi revogado, mas não consegui excluí-lo. Tente de novo.',
    }
  }

  await recordAudit({
    actorId: admin.user.id,
    action: 'deleteInvite',
    target: inviteId,
    ok: deleted,
    detail: deleted ? resultMessage(revoked) : 'invite not found',
  })

  revalidatePath('/dashboard/invites')
  revalidatePath('/dashboard/peers')

  if (!deleted) return { ok: false, error: 'Esse convite já tinha sido excluído.' }
  return { ok: true, message: `Convite excluído — ${droppedAccesses(revoked.affected)}` }
}

/**
 * Only new leases check the cap, and provisiond caches the invite for up to a minute: lowering it
 * never drops the machines already connected.
 */
export async function setInviteMaxLeases(inviteId: string, raw: string): Promise<ActionResult> {
  const admin = await requireAdmin()
  if (!admin.ok) return { ok: false, error: 'Sem permissão.' }

  const maxLeases = parseMaxLeases(raw)
  if (maxLeases === undefined) return { ok: false, error: INVALID_MAX_LEASES }

  let updated: boolean
  try {
    updated = await setInviteMaxLeasesRecord(inviteId, maxLeases)
  } catch (error) {
    console.error('failed to set the invite lease cap', error)
    return { ok: false, error: 'Não consegui alterar o limite do convite.' }
  }

  await recordAudit({
    actorId: admin.user.id,
    action: 'setInviteMaxLeases',
    target: inviteId,
    ok: updated,
    detail: updated ? maxLeasesDetail(maxLeases) : 'invite not found',
  })

  if (!updated) return { ok: false, error: 'Esse convite não existe mais.' }

  revalidatePath('/dashboard/invites')
  return {
    ok: true,
    message: 'Limite alterado. Vale para as próximas conexões em até 1 minuto.',
  }
}

const INVALID_MAX_LEASES = `O limite de conexões precisa ser um número inteiro de 1 a ${MAX_LEASES_LIMIT}, ou ficar vazio para não ter limite.`

function maxLeasesDetail(maxLeases: number | null): string {
  return maxLeases === null ? 'no lease cap' : `max ${maxLeases} leases`
}

function droppedAccesses(count: number): string {
  return count === 1 ? '1 acesso derrubado' : `${count} acessos derrubados`
}
