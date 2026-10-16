import 'server-only'

import { INVITE_KIND, INVITE_STATUS, type ValueOf } from './mongo-names'
import { readProvisiond, writeProvisiond } from './provisiond'

export type InviteKind = ValueOf<typeof INVITE_KIND>

/** GET /admin/invites (testdata/contracts/panel-invites.json). */
export interface InvitesResponse {
  invites: {
    id: string
    label: string
    kind: string
    prefix: string
    status: string
    created_at: string
    max_leases: number | null
    device_count: number
  }[]
}

/** POST /admin/invites (testdata/contracts/panel-create-invite.json). */
export interface CreateInviteRequest {
  label: string
  kind: string
  token_hash: string
  prefix: string
  created_by: string
  max_leases?: number
}

/** PUT /admin/invites/{id}/max-leases (testdata/contracts/panel-max-leases.json). */
export interface MaxLeasesRequest {
  max_leases: number | null
}

export interface Invite {
  id: string
  label: string
  kind: InviteKind
  prefix: string
  active: boolean
  createdAt: Date
  maxLeases: number | null
  deviceCount: number
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

/** Newest first. */
export async function listRecentInvites(): Promise<Invite[]> {
  const { invites } = await readProvisiond<InvitesResponse>('/admin/invites')
  return invites.map((invite) => ({
    id: invite.id,
    label: invite.label,
    // provisiond stores only the two kinds; anything else reads as the original kind.
    kind: isInviteKind(invite.kind) ? invite.kind : INVITE_KIND.individual,
    prefix: invite.prefix,
    active: invite.status === INVITE_STATUS.active,
    createdAt: new Date(invite.created_at),
    maxLeases: invite.max_leases,
    deviceCount: invite.device_count,
  }))
}

function invitePath(inviteId: string): string {
  return `/admin/invites/${encodeURIComponent(inviteId)}`
}

/** Removes the invite document only; its devices keep their history. False when nothing matched. */
export function deleteInvite(inviteId: string): Promise<boolean> {
  return writeProvisiond('DELETE', invitePath(inviteId))
}

/** `null` removes the cap. False when no invite matched. */
export function setInviteMaxLeases(inviteId: string, maxLeases: number | null): Promise<boolean> {
  const body: MaxLeasesRequest = { max_leases: maxLeases }
  return writeProvisiond('PUT', `${invitePath(inviteId)}/max-leases`, body)
}

/** Stores an active invite. */
export async function insertInvite(invite: NewInvite): Promise<void> {
  const body: CreateInviteRequest = {
    label: invite.label,
    kind: invite.kind,
    token_hash: invite.tokenHash,
    prefix: invite.prefix,
    created_by: invite.createdBy,
    ...(invite.maxLeases === null ? {} : { max_leases: invite.maxLeases }),
  }
  if (!(await writeProvisiond('POST', '/admin/invites', body))) {
    throw new Error('provisiond answered 404 to POST /admin/invites')
  }
}
