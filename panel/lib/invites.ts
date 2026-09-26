import { ObjectId } from 'mongodb'
import { collection } from './mongodb'
import { COLLECTION, FIELD, INVITE_KIND, INVITE_STATUS, type ValueOf } from './mongo-names'

const RECENT_INVITES_LIMIT = 100
const LABELS_LIMIT = 200

export type InviteKind = ValueOf<typeof INVITE_KIND>

interface InviteDocument {
  [FIELD.id]: ObjectId
  [FIELD.label]: string
  /** Missing on invites created before kinds existed, which were all individual. */
  [FIELD.kind]?: InviteKind
  [FIELD.tokenHash]: string
  [FIELD.prefix]: string
  [FIELD.status]: ValueOf<typeof INVITE_STATUS>
  /** The fixed admin's id, not an ObjectId. */
  [FIELD.createdBy]: string
  [FIELD.createdAt]: Date
  [FIELD.revokedAt]?: Date
  [FIELD.expiresAt]?: Date
  /** provisiond refuses a new lease past this many; missing means no cap. */
  [FIELD.maxLeases]?: number
}

export interface Invite {
  id: string
  label: string
  kind: InviteKind
  prefix: string
  active: boolean
  createdAt: Date
  maxLeases: number | null
}

export interface NewInvite {
  label: string
  kind: InviteKind
  tokenHash: string
  prefix: string
  createdBy: string
  maxLeases: number | null
}

export function isInviteKind(value: unknown): value is InviteKind {
  return Object.values(INVITE_KIND).includes(value as InviteKind)
}

function invitesCollection() {
  return collection<InviteDocument>(COLLECTION.invites)
}

/** Newest first. */
export async function listRecentInvites(): Promise<Invite[]> {
  const invites = await invitesCollection()
  const docs = await invites
    .find({})
    .sort({ [FIELD.createdAt]: -1 })
    .limit(RECENT_INVITES_LIMIT)
    .toArray()

  return docs.map((doc) => ({
    id: doc[FIELD.id].toHexString(),
    label: doc[FIELD.label],
    kind: doc[FIELD.kind] ?? INVITE_KIND.individual,
    prefix: doc[FIELD.prefix],
    active: doc[FIELD.status] === INVITE_STATUS.active,
    createdAt: doc[FIELD.createdAt],
    maxLeases: doc[FIELD.maxLeases] ?? null,
  }))
}

/** Invite labels keyed by invite id. */
export async function inviteLabelsById(): Promise<Record<string, string>> {
  const invites = await invitesCollection()
  const docs = await invites
    .find({}, { projection: { [FIELD.label]: 1 } })
    .limit(LABELS_LIMIT)
    .toArray()

  return Object.fromEntries(docs.map((doc) => [doc[FIELD.id].toHexString(), doc[FIELD.label]]))
}

/** Removes the invite document only; its devices keep their history. False when nothing matched. */
export async function deleteInvite(inviteId: string): Promise<boolean> {
  if (!ObjectId.isValid(inviteId)) return false
  const invites = await invitesCollection()
  const { deletedCount } = await invites.deleteOne({ [FIELD.id]: new ObjectId(inviteId) })
  return deletedCount === 1
}

/** `null` removes the cap. False when no invite matched. */
export async function setInviteMaxLeases(
  inviteId: string,
  maxLeases: number | null,
): Promise<boolean> {
  if (!ObjectId.isValid(inviteId)) return false
  const invites = await invitesCollection()
  const update =
    maxLeases === null
      ? { $unset: { [FIELD.maxLeases]: '' as const } }
      : { $set: { [FIELD.maxLeases]: maxLeases } }
  const { matchedCount } = await invites.updateOne({ [FIELD.id]: new ObjectId(inviteId) }, update)
  return matchedCount === 1
}

/** Stores an active invite. */
export async function insertInvite(invite: NewInvite): Promise<void> {
  const invites = await invitesCollection()
  await invites.insertOne({
    [FIELD.id]: new ObjectId(),
    [FIELD.label]: invite.label,
    [FIELD.kind]: invite.kind,
    [FIELD.tokenHash]: invite.tokenHash,
    [FIELD.prefix]: invite.prefix,
    [FIELD.status]: INVITE_STATUS.active,
    [FIELD.createdBy]: invite.createdBy,
    [FIELD.createdAt]: new Date(),
    ...(invite.maxLeases === null ? {} : { [FIELD.maxLeases]: invite.maxLeases }),
  })
}
