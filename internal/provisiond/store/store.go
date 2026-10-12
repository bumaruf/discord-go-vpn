// Package store keeps invites, devices and metrics in Mongo. The database is optional: when
// it is down or unset, leases keep working and only the panel loses data.
package store

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Store is what the server reads from and writes to the database, so tests can run without
// Mongo. A server without a database holds a nil store.
type Store interface {
	// FindInvite returns nil, nil when no invite has the hash.
	FindInvite(ctx context.Context, tokenHash string) (*InviteRecord, error)
	// RevokeInvite and ReactivateInvite return ErrInvalidInviteID for an id no invite can have.
	RevokeInvite(ctx context.Context, inviteID string, at time.Time) error
	ReactivateInvite(ctx context.Context, inviteID string) error
	// RegisterDevice returns an empty id, and no error, for an invite id no invite can have.
	RegisterDevice(ctx context.Context, d Device) (deviceID string, err error)
}

var ErrInvalidInviteID = errors.New("invalid invite id")

type Device struct {
	InviteID   string
	Label      string
	OSUser     string
	Origin     string
	LastSeenAt time.Time
}

type UserSample struct {
	Identity string
	DownBps  float64
	UpBps    float64
}

type Session struct {
	LeaseID       string
	InviteID      string
	DeviceID      string
	Label         string
	PublicKey     string
	IP            string
	Origin        string
	RxBytes       uint64
	TxBytes       uint64
	Online        bool
	LastHandshake time.Time // zero when there was none
	UpdatedAt     time.Time
	StartedAt     time.Time // written only by the session's first write
}

type DailyUsage struct {
	Identity      string
	Day           string
	RxBytes       uint64
	TxBytes       uint64
	OnlineSeconds int
}

const (
	connectTimeout         = 10 * time.Second
	serverSelectionTimeout = 5 * time.Second
	// clientRequestTimeout bounds the queries a client's /lease, /renew or /release waits on.
	clientRequestTimeout = 5 * time.Second
	writeTimeout         = 8 * time.Second
)

type Mongo struct {
	client *mongo.Client
	db     *mongo.Database
}

// Open returns nil, and no error, without MONGODB_URI: running without the panel is a
// supported mode.
func Open(ctx context.Context, cfg config.Config) (*Mongo, error) {
	if cfg.MongoURI == "" {
		log.Print("no MONGODB_URI: metrics won't be published")
		return nil, nil
	}

	name := cfg.MongoDatabaseName()

	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().
		ApplyURI(cfg.MongoURI).
		SetServerSelectionTimeout(serverSelectionTimeout).
		SetMaxPoolSize(8))
	if err != nil {
		return nil, err
	}

	// A server that boots during an Atlas outage still serves leases.
	if err := client.Ping(ctx, nil); err != nil {
		log.Printf("the database didn't answer the ping (going on without it for now): %v", err)
	}

	log.Printf("database connected: %s", name)
	return &Mongo{client: client, db: client.Database(name)}, nil
}

func (m *Mongo) Close(ctx context.Context) {
	if m == nil {
		return
	}
	_ = m.client.Disconnect(ctx)
}

func (m *Mongo) FindInvite(ctx context.Context, hash string) (*InviteRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, clientRequestTimeout)
	defer cancel()

	var doc InviteRecord
	err := m.db.Collection(invitesCollection).FindOne(ctx, bson.M{fieldTokenHash: hash}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

func (m *Mongo) RevokeInvite(ctx context.Context, inviteID string, at time.Time) error {
	return m.updateInvite(ctx, inviteID, bson.M{"$set": bson.M{fieldStatus: InviteRevoked, fieldRevokedAt: at}})
}

func (m *Mongo) ReactivateInvite(ctx context.Context, inviteID string) error {
	return m.updateInvite(ctx, inviteID, bson.M{"$set": bson.M{fieldStatus: InviteActive}, "$unset": bson.M{fieldRevokedAt: ""}})
}

func (m *Mongo) updateInvite(ctx context.Context, inviteID string, update bson.M) error {
	oid, err := bson.ObjectIDFromHex(inviteID)
	if err != nil {
		return ErrInvalidInviteID
	}
	return m.updateExisting(ctx, invitesCollection, bson.M{"_id": oid}, update)
}

func (m *Mongo) RegisterDevice(ctx context.Context, d Device) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, clientRequestTimeout)
	defer cancel()
	return registerDevice(ctx, d, m.updateDevice)
}

// deviceCollation compares device names and os users ignoring case, the same collation as the
// devices' unique index the panel creates (panel/lib/indexes.ts). A query without it would not
// use that index, and would miss a device whose name differs only in case.
var deviceCollation = &options.Collation{Locale: "en", Strength: 2}

// updateDevice updates the most recently seen of the matching devices.
func (m *Mongo) updateDevice(ctx context.Context, q deviceQuery) (string, error) {
	var doc struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err := m.db.Collection(devicesCollection).
		FindOneAndUpdate(ctx, q.filter, q.update,
			options.FindOneAndUpdate().
				SetUpsert(q.upsert).
				SetCollation(deviceCollation).
				SetSort(bson.D{{Key: fieldLastSeenAt, Value: -1}}).
				SetReturnDocument(options.After).
				SetProjection(bson.M{"_id": 1}),
		).Decode(&doc)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return "", nil
	case mongo.IsDuplicateKeyError(err):
		return "", errDuplicateDevice
	case err != nil:
		return "", err
	}
	return doc.ID.Hex(), nil
}

func (m *Mongo) AddServerSample(ctx context.Context, at time.Time, downBps, upBps float64) error {
	return m.insert(ctx, samplesCollection, []any{bson.M{
		fieldTimestamp: at,
		fieldMeta:      bson.M{fieldScope: scopeServer},
		fieldDownBps:   downBps,
		fieldUpBps:     upBps,
	}})
}

func (m *Mongo) AddUserSamples(ctx context.Context, at time.Time, samples []UserSample) error {
	if len(samples) == 0 {
		return nil
	}

	docs := make([]any, 0, len(samples))
	for _, s := range samples {
		docs = append(docs, bson.M{
			fieldTimestamp: at,
			fieldMeta:      bson.M{fieldScope: scopeUser, fieldIdentity: s.Identity},
			fieldDownBps:   s.DownBps,
			fieldUpBps:     s.UpBps,
		})
	}
	return m.insert(ctx, samplesCollection, docs)
}

func (m *Mongo) SaveSession(ctx context.Context, s Session) error {
	filter, update := sessionUpsert(s)
	return m.upsert(ctx, sessionsCollection, filter, update)
}

// sessionUpsert stores inviteId as an ObjectId, the type of the invite's _id, or null without
// an invite.
func sessionUpsert(s Session) (filter, update bson.M) {
	status := sessionOffline
	if s.Online {
		status = sessionOnline
	}

	var inviteID any
	if oid, err := bson.ObjectIDFromHex(s.InviteID); err == nil {
		inviteID = oid
	}

	doc := bson.M{
		fieldInviteID:  inviteID,
		fieldDeviceID:  s.DeviceID,
		fieldLabel:     s.Label,
		fieldPublicKey: s.PublicKey,
		fieldIP:        s.IP,
		fieldOriginIP:  s.Origin,
		fieldRxBytes:   s.RxBytes,
		fieldTxBytes:   s.TxBytes,
		fieldStatus:    status,
		fieldUpdatedAt: s.UpdatedAt,
	}
	if !s.LastHandshake.IsZero() {
		doc[fieldLastHandshake] = s.LastHandshake
	}

	return bson.M{"_id": s.LeaseID}, bson.M{"$set": doc, "$setOnInsert": bson.M{fieldStartedAt: s.StartedAt}}
}

func (m *Mongo) AddDailyUsage(ctx context.Context, u DailyUsage) error {
	return m.upsert(ctx, dailyUsageCollection,
		bson.M{"_id": fmt.Sprintf("%s:%s", u.Identity, u.Day)},
		bson.M{
			"$inc": bson.M{
				fieldRxBytes:       u.RxBytes,
				fieldTxBytes:       u.TxBytes,
				fieldOnlineSeconds: u.OnlineSeconds,
			},
			"$setOnInsert": bson.M{fieldIdentity: u.Identity, fieldDay: u.Day},
		})
}

func (m *Mongo) insert(ctx context.Context, collection string, docs []any) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	_, err := m.db.Collection(collection).InsertMany(ctx, docs)
	return err
}

// upsert returns the error so the writer keeps the write queued.
func (m *Mongo) upsert(ctx context.Context, collection string, filter, update any) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	_, err := m.db.Collection(collection).UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Printf("writing to %s: %v", collection, err)
	}
	return err
}

// updateExisting never creates a document, so a made-up id can't create an invite.
func (m *Mongo) updateExisting(ctx context.Context, collection string, filter, update any) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	_, err := m.db.Collection(collection).UpdateOne(ctx, filter, update)
	return err
}
