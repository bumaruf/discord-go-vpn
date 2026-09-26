import type { Db, ObjectId } from 'mongodb'
import type { DeviceDocument } from './devices'
import { COLLECTION, FIELD } from './mongo-names'
import type { SessionDocument } from './sessions'
import type { DailyUsageDocument } from './usage'

/**
 * Joins the usage of a machine that the removal of the global token split in two.
 *
 * Usage is grouped by device id, and a device is keyed by invite, name and os user. Devices the
 * global token registered have a null invite, so the first lease by an invite registered the same
 * machine again and its history restarted. provisiond now adopts such a device instead
 * (`registerDevice`), which only helps a machine that hasn't leased since; this catches up with
 * the ones that already did.
 */

/** One machine's two devices, and what moving the older one's history brought over. */
export interface DeviceMerge {
  /** The device the global token left, deleted once its history moved. */
  from: string
  /** The device the same machine registered under its invite, which keeps the history. */
  into: string
  name: string
  osUser: string | null
  days: number
  sessions: number
}

/** A device without an invite that this pass left alone, and why. */
export interface DeviceLeftAlone {
  id: string
  name: string
  osUser: string | null
  /** The devices under an invite that the machine could be: none, or more than one. */
  candidates: string[]
}

export interface MergeReport {
  merged: DeviceMerge[]
  /** No device under an invite to merge into: provisiond adopts it on the machine's next lease. */
  waiting: DeviceLeftAlone[]
  /** The same name and os user under more than one invite, so which machine it is has to be decided. */
  ambiguous: DeviceLeftAlone[]
}

/** Idempotent: a merged device is gone, so a second run finds nothing left to move. */
export async function mergeDevicesWithoutInvite(db: Db): Promise<MergeReport> {
  const devices = db.collection<DeviceDocument>(COLLECTION.devices)
  const report: MergeReport = { merged: [], waiting: [], ambiguous: [] }

  const orphans = await devices.find({ [FIELD.inviteId]: null }).toArray()

  for (const orphan of orphans) {
    const osUser = orphan[FIELD.osUser] ?? null
    // A null os user matches a missing one, the way provisiond's device key does.
    const candidates = await devices
      .find({
        [FIELD.name]: orphan[FIELD.name],
        [FIELD.osUser]: osUser,
        [FIELD.inviteId]: { $ne: null },
      })
      .toArray()

    const left: DeviceLeftAlone = {
      id: orphan[FIELD.id].toHexString(),
      name: orphan[FIELD.name],
      osUser,
      candidates: candidates.map((c) => c[FIELD.id].toHexString()),
    }

    if (candidates.length === 0) {
      report.waiting.push(left)
    } else if (candidates.length > 1) {
      report.ambiguous.push(left)
    } else {
      report.merged.push(await mergeDevice(db, orphan, candidates[0]))
    }
  }

  return report
}

async function mergeDevice(
  db: Db,
  from: DeviceDocument,
  into: DeviceDocument,
): Promise<DeviceMerge> {
  const fromId = from[FIELD.id].toHexString()
  const intoId = into[FIELD.id].toHexString()

  const days = await moveDailyUsage(db, fromId, intoId)
  const sessions = await db
    .collection<SessionDocument>(COLLECTION.sessions)
    .updateMany({ [FIELD.deviceId]: fromId }, { $set: { [FIELD.deviceId]: intoId } })

  await moveFirstAccess(db, from, into[FIELD.id])
  await db.collection<DeviceDocument>(COLLECTION.devices).deleteOne({ [FIELD.id]: from[FIELD.id] })

  return {
    from: fromId,
    into: intoId,
    name: from[FIELD.name],
    osUser: from[FIELD.osUser] ?? null,
    days,
    sessions: sessions.modifiedCount,
  }
}

/**
 * Adds each day of `fromId` to the same day of `intoId`, which may already have one: both devices
 * recorded traffic on the day the machine changed over. Each day is deleted before it is added, so
 * an interrupted run loses a day rather than counting it twice when it runs again.
 */
async function moveDailyUsage(db: Db, fromId: string, intoId: string): Promise<number> {
  const dailyUsage = db.collection<DailyUsageDocument>(COLLECTION.dailyUsage)
  const rows = await dailyUsage.find({ [FIELD.identity]: fromId }).toArray()

  for (const row of rows) {
    await dailyUsage.deleteOne({ [FIELD.id]: row[FIELD.id] })
    await dailyUsage.updateOne(
      { [FIELD.id]: `${intoId}:${row[FIELD.day]}` },
      {
        $inc: {
          [FIELD.rxBytes]: row[FIELD.rxBytes] ?? 0,
          [FIELD.txBytes]: row[FIELD.txBytes] ?? 0,
          [FIELD.onlineSeconds]: row[FIELD.onlineSeconds] ?? 0,
        },
        $setOnInsert: { [FIELD.identity]: intoId, [FIELD.day]: row[FIELD.day] },
      },
      { upsert: true },
    )
  }

  return rows.length
}

/** The older device holds the machine's first access, so the surviving one takes it over. */
async function moveFirstAccess(db: Db, from: DeviceDocument, intoId: ObjectId): Promise<void> {
  const firstIp = from[FIELD.firstIp]

  await db.collection<DeviceDocument>(COLLECTION.devices).updateOne(
    { [FIELD.id]: intoId },
    {
      $min: { [FIELD.createdAt]: from[FIELD.createdAt] },
      $inc: { [FIELD.accessCount]: from[FIELD.accessCount] ?? 0 },
      ...(firstIp ? { $set: { [FIELD.firstIp]: firstIp } } : {}),
    },
  )
}
