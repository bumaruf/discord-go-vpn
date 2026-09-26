'use server'

import { revalidatePath } from 'next/cache'
import { resultMessage, type ActionResult } from '@/lib/action-result'
import { recordAudit } from '@/lib/audit'
import { disconnectLease } from '@/lib/provisiond'
import { requireAdmin } from '@/lib/require-admin'

/** Temporary: the device reconnects with its invite. Revoking the invite is what's permanent. */
export async function disconnectPeer(leaseId: string): Promise<ActionResult> {
  const admin = await requireAdmin()
  if (!admin.ok) return { ok: false, error: 'Sem permissão.' }

  const result = await disconnectLease(leaseId)

  await recordAudit({
    actorId: admin.user.id,
    action: 'disconnect',
    target: leaseId,
    ok: result.ok,
    detail: resultMessage(result),
  })

  revalidatePath('/dashboard/peers')
  return result
}
