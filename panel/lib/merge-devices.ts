import type { Db, ObjectId } from 'mongodb'
import type { DailyUsageDocument, DeviceDocument, SessionDocument } from './documents'
import { COLLECTION, FIELD } from './mongo-names'

/**
 * Joins the devices of one machine, which used to get a new device, and so a new usage history,
 * whenever it leased through another invite (a replaced invite, a global one, the null invite the
 * removed global token left) or sent its name in another case.
 *
 * A machine is its name and os user, ignoring case. provisiond now adopts the machine's device
 * across invites and cases (`registerDevice`), which only helps a machine that hasn't leased since;
 * this catches up with the ones that already did.
 */

/** One machine's devices: the most recently seen keeps the history of the others. */
export interface MachinePlan {
  into: DeviceDocument
  from: DeviceDocument[]
}

/** A device without an os user whose name more than one os user has, so it can't be placed. */
export interface AmbiguousDevice {
  device: DeviceDocument
  osUsers: string[]
}

export interface MergePlan {
  machines: MachinePlan[]
  ambiguous: AmbiguousDevice[]
}

/** One device merged into its machine's surviving device, and the history that moved. */
export interface DeviceMerge {
  from: string
  into: string
  name: string
  osUser: string | null
  days: number
  sessions: number
}

/** Reads only, so `--dry-run` can show what a merge would do. */
export async function planDeviceMerges(db: Db): Promise<MergePlan> {
  const docs = await db
    .collection<DeviceDocument>(COLLECTION.devices)
    .find({})
    .sort({ [FIELD.lastSeenAt]: -1 })
    .toArray()

  // Machines without a name would all be one, so they are never merged.
  const named = docs.filter((doc) => doc[FIELD.name])
  const groups = new Map<string, DeviceDocument[]>()
  for (const doc of named.filter((d) => d[FIELD.osUser])) {
    const key = machineKey(doc)
    groups.set(key, [...(groups.get(key) ?? []), doc])
  }

  const ambiguous: AmbiguousDevice[] = []
  const withoutOsUser = new Map<string, DeviceDocument[]>()
  for (const doc of named.filter((d) => !d[FIELD.osUser])) {
    // A device from before clients sent an os user belongs to the only os user of its name.
    const owners = [...groups.keys()].filter((key) => key.startsWith(nameKey(doc)))
    if (owners.length === 1) {
      groups.get(owners[0])!.push(doc)
    } else if (owners.length > 1) {
      const osUsers = owners.map((key) => groups.get(key)![0][FIELD.osUser]!)
      ambiguous.push({ device: doc, osUsers })
    } else {
      const key = nameKey(doc)
      withoutOsUser.set(key, [...(withoutOsUser.get(key) ?? []), doc])
    }
  }

  const machines = [...groups.values(), ...withoutOsUser.values()]
    .filter((group) => group.length > 1)
    .map((group) => {
      const [into, ...from] = group.sort(
        (a, b) => b[FIELD.lastSeenAt].getTime() - a[FIELD.lastSeenAt].getTime(),
      )
      return { into, from }
    })

  return { machines, ambiguous }
}

/**
 * Idempotent: a merged device is gone, so a second run finds nothing left to move. The surviving
 * device keeps its invite, the one the machine leased through last.
 */
export async function mergeDevices(db: Db, plan: MergePlan): Promise<DeviceMerge[]> {
  const devices = db.collection<DeviceDocument>(COLLECTION.devices)
  const merged: DeviceMerge[] = []

  for (const machine of plan.machines) {
    for (const from of machine.from) merged.push(await mergeDevice(db, from, machine.into))

    // Set only after the others are gone, since one of them may hold this key in the same invite.
    const osUser = machine.from.find((device) => device[FIELD.osUser])?.[FIELD.osUser]
    if (!machine.into[FIELD.osUser] && osUser) {
      await devices.updateOne(
        { [FIELD.id]: machine.into[FIELD.id] },
        { $set: { [FIELD.osUser]: osUser } },
      )
    }
  }

  return merged
}

/** Name and os user, ignoring case like the devices' unique index. */
function machineKey(doc: DeviceDocument): string {
  return `${nameKey(doc)}${doc[FIELD.osUser]!.toLowerCase()}`
}

function nameKey(doc: DeviceDocument): string {
  return `${doc[FIELD.name].toLowerCase()}\0`
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
