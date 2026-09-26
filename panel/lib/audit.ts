import { ObjectId } from 'mongodb'
import { collection } from './mongodb'
import { AUDIT_ACTION, AUDIT_RESULT, COLLECTION, FIELD, type ValueOf } from './mongo-names'

interface AuditDocument {
  [FIELD.id]: ObjectId
  [FIELD.actorId]: string
  [FIELD.action]: ValueOf<typeof AUDIT_ACTION>
  [FIELD.target]?: string
  [FIELD.at]: Date
  [FIELD.ip]?: string
  [FIELD.result]: ValueOf<typeof AUDIT_RESULT>
  [FIELD.detail]?: string
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
  try {
    const auditLog = await collection<AuditDocument>(COLLECTION.auditLog)
    await auditLog.insertOne({
      // The driver would generate the same id; passing it satisfies the declared `_id: ObjectId`.
      [FIELD.id]: new ObjectId(),
      [FIELD.actorId]: actorId,
      [FIELD.action]: AUDIT_ACTION[action],
      [FIELD.target]: target,
      [FIELD.at]: new Date(),
      [FIELD.result]: ok ? AUDIT_RESULT.ok : AUDIT_RESULT.error,
      [FIELD.detail]: detail,
    })
  } catch (error) {
    console.error('failed to record the audit entry', error)
  }
}
