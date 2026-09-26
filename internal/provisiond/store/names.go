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
)

// Stored values.
const (
	InviteActive  = "active"
	InviteRevoked = "revoked"

	sessionOnline  = "online"
	sessionOffline = "offline"

	scopeServer = "server"
	scopeUser   = "user"

	// unnamedDevice is the name of a device whose client sent no label.
	unnamedDevice = ""
)

// InviteRecord is the part of an invite document the authenticator reads. Its bson tags
// are field names too.
type InviteRecord struct {
	ID        bson.ObjectID `bson:"_id"`
	Status    string        `bson:"status"`
	ExpiresAt *time.Time    `bson:"expiresAt"`
	// MaxLeases is missing on invites without a cap, and on those created before caps existed.
	MaxLeases *int `bson:"maxLeases"`
}
