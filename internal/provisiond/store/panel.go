package store

import (
	"cmp"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// PanelData is what the panel reads and writes through provisiond, which keeps the only pool of
// connections to the database.
type PanelData interface {
	// ListInvites returns the newest invites first, each with its device count.
	ListInvites(ctx context.Context, limit int) ([]InviteSummary, error)
	InviteLabels(ctx context.Context, limit int) (map[string]string, error)
	// ListDevices returns the most recently seen devices first.
	ListDevices(ctx context.Context, limit int) ([]DeviceSummary, error)
	// UsageByDay sums the daily usage from day from on, oldest day first: of every device, or of
	// one when identity isn't empty. Days without traffic are absent.
	UsageByDay(ctx context.Context, from, identity string) ([]DayUsage, error)
	InsertInvite(ctx context.Context, invite NewInvite) error
	// DeleteInvite and SetInviteMaxLeases return false when no invite has the id. A nil max
	// removes the cap.
	DeleteInvite(ctx context.Context, inviteID string) (bool, error)
	SetInviteMaxLeases(ctx context.Context, inviteID string, max *int) (bool, error)
	InsertAudit(ctx context.Context, entry AuditEntry) error
}

type InviteSummary struct {
	ID          string
	Label       string
	Kind        string
	Prefix      string
	Status      string
	CreatedAt   time.Time
	MaxLeases   *int
	DeviceCount int
}

type DeviceSummary struct {
	ID          string
	InviteID    string // empty for a device without invite
	Name        string
	OSUser      string
	LastIP      string
	CreatedAt   time.Time
	LastSeenAt  time.Time
	AccessCount int
}

type DayUsage struct {
	Day     string
	RxBytes int64
	TxBytes int64
}

type NewInvite struct {
	Label     string
	Kind      string
	TokenHash string
	Prefix    string
	CreatedBy string
	CreatedAt time.Time
	MaxLeases *int
}

type AuditEntry struct {
	ActorID string
	Action  string
	Target  string
	OK      bool
	Detail  string
	At      time.Time
}

// panelReadTimeout bounds a panel query: the admin is waiting on the page.
const panelReadTimeout = 5 * time.Second

func (m *Mongo) ListInvites(ctx context.Context, limit int) ([]InviteSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, panelReadTimeout)
	defer cancel()

	cursor, err := m.db.Collection(invitesCollection).Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: fieldCreatedAt, Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	var docs []struct {
		ID        bson.ObjectID `bson:"_id"`
		Label     string        `bson:"label"`
		Kind      string        `bson:"kind"`
		Prefix    string        `bson:"prefix"`
		Status    string        `bson:"status"`
		CreatedAt time.Time     `bson:"createdAt"`
		MaxLeases *int          `bson:"maxLeases"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	ids := make([]bson.ObjectID, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	counts, err := m.countDevicesByInvite(ctx, ids)
	if err != nil {
		return nil, err
	}

	invites := make([]InviteSummary, len(docs))
	for i, d := range docs {
		invites[i] = InviteSummary{
			ID:          d.ID.Hex(),
			Label:       d.Label,
			Kind:        cmp.Or(d.Kind, InviteIndividual),
			Prefix:      d.Prefix,
			Status:      d.Status,
			CreatedAt:   d.CreatedAt,
			MaxLeases:   d.MaxLeases,
			DeviceCount: counts[d.ID],
		}
	}
	return invites, nil
}

func (m *Mongo) countDevicesByInvite(ctx context.Context, ids []bson.ObjectID) (map[bson.ObjectID]int, error) {
	counts := make(map[bson.ObjectID]int)
	if len(ids) == 0 {
		return counts, nil
	}

	cursor, err := m.db.Collection(devicesCollection).Aggregate(ctx, bson.A{
		bson.M{"$match": bson.M{fieldInviteID: bson.M{"$in": ids}}},
		bson.M{"$group": bson.M{"_id": "$" + fieldInviteID, "total": bson.M{"$sum": 1}}},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID    bson.ObjectID `bson:"_id"`
		Total int           `bson:"total"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.ID] = r.Total
	}
	return counts, nil
}

func (m *Mongo) InviteLabels(ctx context.Context, limit int) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, panelReadTimeout)
	defer cancel()

	cursor, err := m.db.Collection(invitesCollection).Find(ctx, bson.M{},
		options.Find().SetProjection(bson.M{fieldLabel: 1}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	var docs []struct {
		ID    bson.ObjectID `bson:"_id"`
		Label string        `bson:"label"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	labels := make(map[string]string, len(docs))
	for _, d := range docs {
		labels[d.ID.Hex()] = d.Label
	}
	return labels, nil
}

func (m *Mongo) ListDevices(ctx context.Context, limit int) ([]DeviceSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, panelReadTimeout)
	defer cancel()

	cursor, err := m.db.Collection(devicesCollection).Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: fieldLastSeenAt, Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	var docs []struct {
		ID          bson.ObjectID  `bson:"_id"`
		InviteID    *bson.ObjectID `bson:"inviteId"`
		Name        string         `bson:"name"`
		OSUser      *string        `bson:"osUser"`
		LastIP      string         `bson:"lastIp"`
		CreatedAt   time.Time      `bson:"createdAt"`
		LastSeenAt  time.Time      `bson:"lastSeenAt"`
		AccessCount int            `bson:"accessCount"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	devices := make([]DeviceSummary, len(docs))
	for i, d := range docs {
		devices[i] = DeviceSummary{
			ID:          d.ID.Hex(),
			Name:        d.Name,
			LastIP:      d.LastIP,
			CreatedAt:   d.CreatedAt,
			LastSeenAt:  d.LastSeenAt,
			AccessCount: d.AccessCount,
		}
		if d.InviteID != nil {
			devices[i].InviteID = d.InviteID.Hex()
		}
		if d.OSUser != nil {
			devices[i].OSUser = *d.OSUser
		}
	}
	return devices, nil
}

func (m *Mongo) UsageByDay(ctx context.Context, from, identity string) ([]DayUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, panelReadTimeout)
	defer cancel()

	match := bson.M{fieldDay: bson.M{"$gte": from}}
	if identity != "" {
		match[fieldIdentity] = identity
	}
	cursor, err := m.db.Collection(dailyUsageCollection).Aggregate(ctx, bson.A{
		bson.M{"$match": match},
		bson.M{"$group": bson.M{
			"_id":        "$" + fieldDay,
			fieldRxBytes: bson.M{"$sum": "$" + fieldRxBytes},
			fieldTxBytes: bson.M{"$sum": "$" + fieldTxBytes},
		}},
		bson.M{"$sort": bson.M{"_id": 1}},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Day     string `bson:"_id"`
		RxBytes int64  `bson:"rxBytes"`
		TxBytes int64  `bson:"txBytes"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	days := make([]DayUsage, len(rows))
	for i, r := range rows {
		days[i] = DayUsage(r)
	}
	return days, nil
}

func (m *Mongo) InsertInvite(ctx context.Context, invite NewInvite) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	doc := bson.M{
		fieldLabel:     invite.Label,
		fieldKind:      invite.Kind,
		fieldTokenHash: invite.TokenHash,
		fieldPrefix:    invite.Prefix,
		fieldStatus:    InviteActive,
		fieldCreatedBy: invite.CreatedBy,
		fieldCreatedAt: invite.CreatedAt,
	}
	if invite.MaxLeases != nil {
		doc[fieldMaxLeases] = *invite.MaxLeases
	}
	_, err := m.db.Collection(invitesCollection).InsertOne(ctx, doc)
	return err
}

func (m *Mongo) DeleteInvite(ctx context.Context, inviteID string) (bool, error) {
	oid, err := bson.ObjectIDFromHex(inviteID)
	if err != nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	res, err := m.db.Collection(invitesCollection).DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return false, err
	}
	return res.DeletedCount == 1, nil
}

func (m *Mongo) SetInviteMaxLeases(ctx context.Context, inviteID string, max *int) (bool, error) {
	oid, err := bson.ObjectIDFromHex(inviteID)
	if err != nil {
		return false, nil
	}
	update := bson.M{"$unset": bson.M{fieldMaxLeases: ""}}
	if max != nil {
		update = bson.M{"$set": bson.M{fieldMaxLeases: *max}}
	}

	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	res, err := m.db.Collection(invitesCollection).UpdateOne(ctx, bson.M{"_id": oid}, update)
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

func (m *Mongo) InsertAudit(ctx context.Context, e AuditEntry) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	result := auditError
	if e.OK {
		result = auditOK
	}
	_, err := m.db.Collection(auditLogCollection).InsertOne(ctx, bson.M{
		fieldActorID: e.ActorID,
		fieldAction:  e.Action,
		fieldTarget:  e.Target,
		fieldAt:      e.At,
		fieldResult:  result,
		fieldDetail:  e.Detail,
	})
	return err
}
