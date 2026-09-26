// The Mongo names, shared with provisiond (internal/provisiond/store/names.go).

const STORED_NAMES = {
  collection: {
    invites: 'invites',
    devices: 'devices',
    sessions: 'sessions',
    samples: 'samples',
    dailyUsage: 'daily_usage',
    auditLog: 'audit_log',
    loginAttempts: 'login_attempts',
  },
  field: {
    id: '_id',
    tokenHash: 'tokenHash',
    status: 'status',
    kind: 'kind',
    label: 'label',
    prefix: 'prefix',
    createdBy: 'createdBy',
    createdAt: 'createdAt',
    revokedAt: 'revokedAt',
    expiresAt: 'expiresAt',
    maxLeases: 'maxLeases',
    attempts: 'attempts',
    inviteId: 'inviteId',
    deviceId: 'deviceId',
    name: 'name',
    osUser: 'osUser',
    firstIp: 'firstIp',
    lastIp: 'lastIp',
    lastSeenAt: 'lastSeenAt',
    accessCount: 'accessCount',
    publicKey: 'pubKey',
    ip: 'ip',
    originIp: 'originIp',
    startedAt: 'startedAt',
    lastHandshake: 'lastHandshake',
    updatedAt: 'updatedAt',
    rxBytes: 'rxBytes',
    txBytes: 'txBytes',
    timestamp: 'ts',
    meta: 'meta',
    scope: 'scope',
    identity: 'identity',
    downBps: 'downBps',
    upBps: 'upBps',
    day: 'day',
    onlineSeconds: 'onlineSeconds',
    actorId: 'actorId',
    action: 'action',
    target: 'target',
    at: 'at',
    result: 'result',
    detail: 'detail',
  },
  inviteStatus: { active: 'active', revoked: 'revoked' },
  inviteKind: { individual: 'individual', global: 'global' },
  sessionStatus: { online: 'online', offline: 'offline' },
  sampleScope: { server: 'server', user: 'user' },
  auditResult: { ok: 'ok', error: 'error' },
  auditAction: {
    createInvite: 'create_invite',
    revokeInvite: 'revoke_invite',
    reactivateInvite: 'reactivate_invite',
    deleteInvite: 'delete_invite',
    setInviteMaxLeases: 'set_invite_max_leases',
    disconnect: 'disconnect',
  },
} as const

export const COLLECTION = STORED_NAMES.collection

export const FIELD = STORED_NAMES.field

// Stored values.

export const INVITE_STATUS = STORED_NAMES.inviteStatus

/** Organizational only: provisiond treats both kinds alike. */
export const INVITE_KIND = STORED_NAMES.inviteKind

export const SESSION_STATUS = STORED_NAMES.sessionStatus

export const SAMPLE_SCOPE = STORED_NAMES.sampleScope

export const AUDIT_RESULT = STORED_NAMES.auditResult

export const AUDIT_ACTION = STORED_NAMES.auditAction

export type ValueOf<T> = T[keyof T]

/** A `$group`/`$sum` reference to a stored field. */
export function fieldRef(field: ValueOf<typeof FIELD>): `$${typeof field}` {
  return `$${field}`
}
