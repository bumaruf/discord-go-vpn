import { ObjectId } from 'mongodb'
import { collection } from './mongodb'
import { COLLECTION, FIELD, fieldRef } from './mongo-names'

const DEVICES_LIMIT = 500

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

/** Dates as ISO strings: the list is rendered by a client component. */
export interface Device {
  id: string
  inviteId?: string
  name: string
  osUser?: string
  lastIp?: string
  firstSeenAt: string
  lastSeenAt: string
  accessCount: number
}

function devicesCollection() {
  return collection<DeviceDocument>(COLLECTION.devices)
}

/**
 * Every registered machine, the most recently seen first — including the ones not connected now,
 * which hold the usage history of their leases.
 */
export async function listDevices(): Promise<Device[]> {
  const devices = await devicesCollection()
  const docs = await devices
    .find({})
    .sort({ [FIELD.lastSeenAt]: -1 })
    .limit(DEVICES_LIMIT)
    .toArray()

  return docs.map((doc) => ({
    id: doc[FIELD.id].toHexString(),
    inviteId: doc[FIELD.inviteId]?.toHexString(),
    name: doc[FIELD.name],
    osUser: doc[FIELD.osUser] ?? undefined,
    lastIp: doc[FIELD.lastIp],
    firstSeenAt: doc[FIELD.createdAt].toISOString(),
    lastSeenAt: doc[FIELD.lastSeenAt].toISOString(),
    accessCount: doc[FIELD.accessCount] ?? 0,
  }))
}

/** How many devices each invite has, keyed by invite id; invites without devices are absent. */
export async function countDevicesByInvite(inviteIds: string[]): Promise<Map<string, number>> {
  if (inviteIds.length === 0) return new Map()

  const devices = await devicesCollection()
  const counts = await devices
    .aggregate<{ _id: unknown; total: number }>([
      { $match: { [FIELD.inviteId]: { $in: inviteIds.map((id) => new ObjectId(id)) } } },
      { $group: { _id: fieldRef(FIELD.inviteId), total: { $sum: 1 } } },
    ])
    .toArray()

  return new Map(counts.map((count) => [String(count._id), count.total]))
}
