package store

import (
	"cmp"
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// errDuplicateDevice means a device write hit the devices' unique index, which the panel creates.
var errDuplicateDevice = errors.New("duplicate device")

// deviceQuery is one find-and-update on the devices collection.
type deviceQuery struct {
	filter bson.M
	update bson.M
	upsert bool
}

// updateDevice runs q and returns the id of the device it updated or inserted, or empty when
// nothing matched.
type updateDevice func(ctx context.Context, q deviceQuery) (id string, err error)

// deviceKey identifies a device: its name and os user, within its invite.
type deviceKey struct {
	invite any // bson.ObjectID, or nil on a device the removed global token left behind
	name   string
	osUser any // string, or nil when the client sent none
}

// registerDevice records an access of d's device and returns its id, or empty for an invite id
// no invite can have.
func registerDevice(ctx context.Context, d Device, update updateDevice) (string, error) {
	invite, err := bson.ObjectIDFromHex(d.InviteID)
	if err != nil {
		return "", nil
	}
	key := deviceKey{invite: invite, name: cmp.Or(d.Label, unnamedDevice)}
	if d.OSUser != "" {
		key.osUser = d.OSUser
	}

	id, err := findOrInsertDevice(ctx, d, key, update)
	if errors.Is(err, errDuplicateDevice) {
		// A concurrent lease of the same device wrote it first.
		id, err = update(ctx, deviceQuery{filter: key.filter(), update: deviceAccess(d)})
		if err == nil && id == "" {
			err = errors.New("the device that conflicted with the write wasn't found")
		}
	}
	return id, err
}

// findOrInsertDevice records the access on the device key matches, or else on one still
// registered under an older key, which the same write brings to key. A device is inserted only
// when no key finds it.
func findOrInsertDevice(ctx context.Context, d Device, key deviceKey, update updateDevice) (string, error) {
	for _, candidate := range append([]deviceKey{key}, key.olderKeys()...) {
		id, err := update(ctx, deviceQuery{filter: candidate.filter(), update: key.adopt(candidate, d)})
		if err != nil || id != "" {
			return id, err
		}
	}
	return update(ctx, deviceQuery{filter: key.filter(), update: key.insert(d), upsert: true})
}

// olderKeys are the keys the same machine may still be registered under, the most specific
// first: devices registered before clients sent an os user have none, and devices the removed
// global token left have a null invite, since it identified nobody. Adopting the latter is what
// keeps a machine's usage history, which is grouped by device id, whole across that removal.
func (k deviceKey) olderKeys() []deviceKey {
	older := []deviceKey{}
	if k.osUser != nil {
		older = append(older, deviceKey{invite: k.invite, name: k.name})
	}
	older = append(older, deviceKey{name: k.name, osUser: k.osUser})
	if k.osUser != nil {
		older = append(older, deviceKey{name: k.name})
	}
	return older
}

// adopt records the access and fills in what older lacks of k, so the next lease finds the
// device by k itself. Adopting k is only recording the access.
func (k deviceKey) adopt(older deviceKey, d Device) bson.M {
	update := deviceAccess(d)
	set := update["$set"].(bson.M)
	if older.invite == nil {
		set[fieldInviteID] = k.invite
	}
	if older.osUser == nil && k.osUser != nil {
		set[fieldOSUser] = k.osUser
	}
	return update
}

func (k deviceKey) insert(d Device) bson.M {
	insert := deviceAccess(d)
	insert["$setOnInsert"] = bson.M{
		fieldInviteID:  k.invite,
		fieldName:      k.name,
		fieldOSUser:    k.osUser,
		fieldFirstIP:   d.Origin,
		fieldCreatedAt: d.LastSeenAt,
	}
	return insert
}

// filter also matches a missing field where the key part is nil.
func (k deviceKey) filter() bson.M {
	return bson.M{fieldInviteID: k.invite, fieldName: k.name, fieldOSUser: k.osUser}
}

// deviceAccess sets the last access and counts it.
func deviceAccess(d Device) bson.M {
	set := bson.M{fieldLastIP: d.Origin, fieldLastSeenAt: d.LastSeenAt}
	return bson.M{"$set": set, "$inc": bson.M{fieldAccessCount: 1}}
}
