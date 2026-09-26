import type { Db } from 'mongodb'
import { COLLECTION, FIELD } from './mongo-names'

const SAMPLES_TTL_SECONDS = 48 * 60 * 60

/** Idempotent, so `npm run setup` can run again; returns the lines the script prints. */
export async function ensureIndexes(db: Db): Promise<string[]> {
  const { invites, devices, sessions, dailyUsage, auditLog, loginAttempts } = COLLECTION
  const done: string[] = []

  // provisiond looks invites up by hash on every lease request.
  await db.collection(invites).createIndex({ [FIELD.tokenHash]: 1 }, { unique: true })
  await db.collection(invites).createIndex({ [FIELD.status]: 1, [FIELD.createdAt]: -1 })
  done.push(`${invites}.${FIELD.tokenHash} (unique), ${invites}.${FIELD.status}+${FIELD.createdAt}`)

  // Unique because simultaneous leases from one computer would otherwise create two devices.
  await db
    .collection(devices)
    .createIndex({ [FIELD.inviteId]: 1, [FIELD.name]: 1, [FIELD.osUser]: 1 }, { unique: true })
  await db.collection(devices).createIndex({ [FIELD.lastSeenAt]: -1 })
  done.push(
    `${devices}.${FIELD.inviteId}+${FIELD.name}+${FIELD.osUser} (unique), ${devices}.${FIELD.lastSeenAt}`,
  )

  await db.collection(sessions).createIndex({ [FIELD.inviteId]: 1, [FIELD.updatedAt]: -1 })
  await db.collection(sessions).createIndex({ [FIELD.deviceId]: 1, [FIELD.startedAt]: -1 })
  await db.collection(sessions).createIndex({ [FIELD.updatedAt]: -1 })
  done.push(
    `${sessions}.${FIELD.inviteId}+${FIELD.updatedAt}, ${sessions}.${FIELD.deviceId}+${FIELD.startedAt}`,
  )

  await db.collection(dailyUsage).createIndex({ [FIELD.identity]: 1, [FIELD.day]: -1 })
  done.push(`${dailyUsage}.${FIELD.identity}+${FIELD.day}`)

  await db.collection(auditLog).createIndex({ [FIELD.at]: -1 })
  done.push(`${auditLog}.${FIELD.at}`)

  await db
    .collection(loginAttempts)
    .createIndex({ [FIELD.expiresAt]: 1 }, { expireAfterSeconds: 0 })
  done.push(`${loginAttempts}.${FIELD.expiresAt} (TTL)`)

  done.push(await ensureSamplesCollection(db))
  return done
}

/**
 * A time series compresses the thousands of small daily samples that a regular collection would
 * index one by one. Its documents can't be updated, which samples never need.
 */
async function ensureSamplesCollection(db: Db): Promise<string> {
  const { samples } = COLLECTION
  const exists = await db.listCollections({ name: samples }).hasNext()
  if (exists) return `${samples} (already exists)`

  await db.createCollection(samples, {
    timeseries: {
      timeField: FIELD.timestamp,
      metaField: FIELD.meta,
      granularity: 'seconds',
    },
    expireAfterSeconds: SAMPLES_TTL_SECONDS,
  })
  return `${samples} (time series, TTL 48h)`
}
