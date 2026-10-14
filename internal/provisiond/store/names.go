package store

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The Mongo names, shared with the panel (panel/lib/mongo-names.ts).

const (
	invitesCollection    = "invites"
	devicesCollection    = "devices"
	samplesCollection    = "samples"
	sessionsCollection   = "sessions"
	dailyUsageCollection = "daily_usage"
	auditLogCollection   = "audit_log"
)

const (
	fieldTokenHash     = "tokenHash"
	fieldStatus        = "status"
	fieldRevokedAt     = "revokedAt"
	fieldInviteID      = "inviteId"
	fieldDeviceID      = "deviceId"
	fieldName          = "name"
	fieldOSUser        = "osUser"
	fieldLabel         = "label"
	fieldFirstIP       = "firstIp"
	fieldLastIP        = "lastIp"
	fieldLastSeenAt    = "lastSeenAt"
	fieldCreatedAt     = "createdAt"
	fieldAccessCount   = "accessCount"
	fieldTimestamp     = "ts"
	fieldMeta          = "meta"
	fieldScope         = "scope"
	fieldIdentity      = "identity"
	fieldDownBps       = "downBps"
	fieldUpBps         = "upBps"
	fieldPublicKey     = "pubKey"
	fieldIP            = "ip"
	fieldOriginIP      = "originIp"
	fieldRxBytes       = "rxBytes"
	fieldTxBytes       = "txBytes"
	fieldUpdatedAt     = "updatedAt"
	fieldLastHandshake = "lastHandshake"
	fieldStartedAt     = "startedAt"
	fieldDay           = "day"
	fieldOnlineSeconds = "onlineSeconds"
	fieldKind          = "kind"
	fieldPrefix        = "prefix"
	fieldCreatedBy     = "createdBy"
	fieldMaxLeases     = "maxLeases"
	fieldActorID       = "actorId"
	fieldAction        = "action"
	fieldTarget        = "target"
	fieldAt            = "at"
	fieldResult        = "result"
	fieldDetail        = "detail"
)

// Stored values.
const (
	InviteActive  = "active"
	InviteRevoked = "revoked"

	// InviteIndividual is also the kind of invites stored before kinds existed.
	InviteIndividual = "individual"
	InviteGlobal     = "global"

	auditOK    = "ok"
	auditError = "error"

	sessionOnline  = "online"
	sessionOffline = "offline"

	scopeServer = "server"
	scopeUser   = "user"

	// unnamedDevice is the name of a device whose client sent no label.
	unnamedDevice = ""
)

// AuditActions are the actions the audit log stores; the panel sends one with each entry.
var AuditActions = []string{
	"create_invite",
	"revoke_invite",
	"reactivate_invite",
	"delete_invite",
	"set_invite_max_leases",
	"disconnect",
}

// InviteRecord is the part of an invite document the authenticator reads. Its bson tags
// are field names too.
type InviteRecord struct {
	ID        bson.ObjectID `bson:"_id"`
	Status    string        `bson:"status"`
	ExpiresAt *time.Time    `bson:"expiresAt"`
	// MaxLeases is missing on invites without a cap, and on those created before caps existed.
	MaxLeases *int `bson:"maxLeases"`
}
