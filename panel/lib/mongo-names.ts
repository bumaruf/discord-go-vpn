// The Mongo names the Node scripts and the invite and audit wire values use. provisiond's
// internal/provisiond/store/names.go holds the full set, and every name here must match it.

const STORED_NAMES = {
  collection: {
    invites: 'invites',
    devices: 'devices',
    sessions: 'sessions',
    samples: 'samples',
    dailyUsage: 'daily_usage',
    auditLog: 'audit_log',
  },
  field: {
    id: '_id',
    tokenHash: 'tokenHash',
    status: 'status',
    label: 'label',
    createdAt: 'createdAt',
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
    identity: 'identity',
    day: 'day',
    onlineSeconds: 'onlineSeconds',
    at: 'at',
  },
  inviteStatus: { active: 'active', revoked: 'revoked' },
  inviteKind: { individual: 'individual', global: 'global' },
  sessionStatus: { online: 'online', offline: 'offline' },
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

export const AUDIT_ACTION = STORED_NAMES.auditAction

export type ValueOf<T> = T[keyof T]
