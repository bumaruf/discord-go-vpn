import type { ObjectId } from 'mongodb'
import { collection } from './mongodb'
import { COLLECTION, FIELD, SESSION_STATUS, type ValueOf } from './mongo-names'

const SESSIONS_LIMIT = 200

/** Declares only what the collector writes (internal/provisiond/store, `SaveSession`). */
export interface SessionDocument {
  /** The lease id as provisiond knows it. */
  [FIELD.id]: string
  [FIELD.inviteId]: ObjectId | null
  [FIELD.deviceId]?: string
  [FIELD.label]?: string
  [FIELD.publicKey]: string
  [FIELD.ip]: string
  [FIELD.originIp]?: string
  [FIELD.startedAt]: Date
  [FIELD.lastHandshake]?: Date
  [FIELD.updatedAt]?: Date
  [FIELD.rxBytes]: number
  [FIELD.txBytes]: number
  [FIELD.status]: ValueOf<typeof SESSION_STATUS>
}

export interface Session {
  leaseId: string
  inviteId?: string
  deviceId?: string
  label?: string
  publicKey: string
  ip: string
  originIp?: string
  lastHandshake?: Date
  rxBytes: number
  txBytes: number
  online: boolean
}

export async function findSessionsUpdatedSince(since: Date): Promise<Session[]> {
  const sessions = await collection<SessionDocument>(COLLECTION.sessions)
  const docs = await sessions
    .find({ [FIELD.updatedAt]: { $gte: since } })
    .limit(SESSIONS_LIMIT)
    .toArray()

  return docs.map((doc) => ({
    leaseId: String(doc[FIELD.id]),
    inviteId: doc[FIELD.inviteId]?.toHexString(),
    deviceId: doc[FIELD.deviceId],
    label: doc[FIELD.label],
    publicKey: doc[FIELD.publicKey],
    ip: doc[FIELD.ip],
    originIp: doc[FIELD.originIp],
    lastHandshake: doc[FIELD.lastHandshake],
    rxBytes: doc[FIELD.rxBytes],
    txBytes: doc[FIELD.txBytes],
    online: doc[FIELD.status] === SESSION_STATUS.online,
  }))
}
