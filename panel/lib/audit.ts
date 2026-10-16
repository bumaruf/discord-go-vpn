import 'server-only'

import { AUDIT_ACTION } from './mongo-names'
import { writeProvisiond } from './provisiond'

/** POST /admin/audit (testdata/contracts/panel-audit.json). */
export interface AuditRequest {
  actor_id: string
  action: string
  target: string
  ok: boolean
  detail: string
}

export type AuditAction = keyof typeof AUDIT_ACTION

export interface AuditEntry {
  actorId: string
  action: AuditAction
  target: string
  ok: boolean
  detail: string
}

/** Never throws: a lost audit row must not undo the action or hide its result from the admin. */
export async function recordAudit({ actorId, action, target, ok, detail }: AuditEntry) {
  const body: AuditRequest = { actor_id: actorId, action: AUDIT_ACTION[action], target, ok, detail }
  try {
    await writeProvisiond('POST', '/admin/audit', body)
  } catch (error) {
    console.error('failed to record the audit entry', error)
  }
}
