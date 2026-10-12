import type { CollationOptions, Db } from 'mongodb'
import { COLLECTION, FIELD } from './mongo-names'

const SAMPLES_TTL_SECONDS = 48 * 60 * 60

/** Case-insensitive, like the queries provisiond's `deviceCollation` sends. */
const DEVICE_COLLATION: CollationOptions = { locale: 'en', strength: 2 }
const DEVICE_KEY_INDEX = 'device_key_case_insensitive'
/** The name Mongo gave the case-sensitive index this one replaced. */
const OLD_DEVICE_KEY_INDEX = `${FIELD.inviteId}_1_${FIELD.name}_1_${FIELD.osUser}_1`

/** Idempotent, so `npm run setup` can run again; returns the lines the script prints. */
export async function ensureIndexes(db: Db): Promise<string[]> {
  const { invites, devices, sessions, dailyUsage, auditLog, loginAttempts } = COLLECTION
  const done: string[] = []

  // provisiond looks invites up by hash on every lease request.
  await db.collection(invites).createIndex({ [FIELD.tokenHash]: 1 }, { unique: true })
  await db.collection(invites).createIndex({ [FIELD.status]: 1, [FIELD.createdAt]: -1 })
  done.push(`${invites}.${FIELD.tokenHash} (unique), ${invites}.${FIELD.status}+${FIELD.createdAt}`)

  await ensureDeviceKeyIndex(db)
  await db.collection(devices).createIndex({ [FIELD.lastSeenAt]: -1 })
  done.push(
    `${devices}.${FIELD.inviteId}+${FIELD.name}+${FIELD.osUser} (unique, case-insensitive), ${devices}.${FIELD.lastSeenAt}`,
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
 * Unique because simultaneous leases from one computer would otherwise create two devices. The
 * case-sensitive index it replaces is dropped only once this one exists, so the devices are never
 * left without one.
 */
async function ensureDeviceKeyIndex(db: Db): Promise<void> {
  const devices = db.collection(COLLECTION.devices)

  try {
    await devices.createIndex(
      { [FIELD.inviteId]: 1, [FIELD.name]: 1, [FIELD.osUser]: 1 },
      { unique: true, collation: DEVICE_COLLATION, name: DEVICE_KEY_INDEX },
    )
  } catch (error) {
    if (!isDuplicateKey(error)) throw error
    throw new Error(
      'devices differing only in case share a key: run npm run merge-devices, then setup again',
    )
  }

  if (await devices.indexExists(OLD_DEVICE_KEY_INDEX)) await devices.dropIndex(OLD_DEVICE_KEY_INDEX)
}

function isDuplicateKey(error: unknown): boolean {
  return typeof error === 'object' && error !== null && 'code' in error && error.code === 11000
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
