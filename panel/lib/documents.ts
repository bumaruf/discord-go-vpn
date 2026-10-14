import type { ObjectId } from 'mongodb'
import { FIELD, SESSION_STATUS, type ValueOf } from './mongo-names'

// The stored documents the Node scripts (npm run merge-devices) read and write. The panel itself
// reads the database only through provisiond.

/** A machine, identified by invite, computer name and OS user; a device without invite has null. */
export interface DeviceDocument {
  [FIELD.id]: ObjectId
  [FIELD.inviteId]: ObjectId | null
  [FIELD.name]: string
  /** Missing on devices registered before provisiond sent it. */
  [FIELD.osUser]?: string | null
  [FIELD.firstIp]?: string
  [FIELD.lastIp]?: string
  [FIELD.createdAt]: Date
  [FIELD.lastSeenAt]: Date
  [FIELD.accessCount]: number
}

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

export interface DailyUsageDocument {
  /** `<identity>:<YYYY-MM-DD>`. */
  [FIELD.id]: string
  /** The device id; `peer:<key>` for the legacy global token. */
  [FIELD.identity]: string
  [FIELD.day]: string
  [FIELD.rxBytes]: number
  [FIELD.txBytes]: number
  [FIELD.onlineSeconds]: number
}
