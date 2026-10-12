package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var deviceTestTime = time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

const deviceTestInvite = "65f1a2b3c4d5e6f7a8b9c0d1"

type deviceReply struct {
	id  string
	err error
}

// fakeDevices records each query and answers it with the next reply, or with no match once
// the replies run out.
type fakeDevices struct {
	replies []deviceReply
	queries []deviceQuery
}

func (f *fakeDevices) update(_ context.Context, q deviceQuery) (string, error) {
	f.queries = append(f.queries, q)
	if len(f.replies) == 0 {
		return "", nil
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return r.id, r.err
}

func TestRegisterDevice(t *testing.T) {
	inviteOID, err := bson.ObjectIDFromHex(deviceTestInvite)
	if err != nil {
		t.Fatal(err)
	}
	errStore := errors.New("server selection error")

	access := func(set bson.M) bson.M {
		s := bson.M{fieldLastIP: "198.51.100.7", fieldLastSeenAt: deviceTestTime}
		for k, v := range set {
			s[k] = v
		}
		return bson.M{"$set": s, "$inc": bson.M{fieldAccessCount: 1}}
	}
	insert := func(inviteID bson.ObjectID, name string, osUser any) bson.M {
		u := access(nil)
		u["$setOnInsert"] = bson.M{
			fieldInviteID:  inviteID,
			fieldName:      name,
			fieldOSUser:    osUser,
			fieldFirstIP:   "198.51.100.7",
			fieldCreatedAt: deviceTestTime,
		}
		return u
	}
	withInvite := func(filter bson.M) bson.M {
		filter[fieldInviteID] = inviteOID
		return filter
	}
	// anyInvite is the filter of a key that finds the machine under whichever invite it has.
	anyInvite := func(filter bson.M) bson.M { return filter }

	cases := []struct {
		name    string
		device  Device
		replies []deviceReply
		want    []deviceQuery
		wantID  string
		wantErr bool
	}{
		{
			name:    "an invite without an os user looks the device up by invite, name and a null os user",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop"},
			replies: []deviceReply{{id: "d1"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(nil)},
			},
			wantID: "d1",
		},
		{
			name:   "a lease without an invite registers nothing",
			device: Device{Label: "laptop", OSUser: "ana"},
		},
		{
			name:   "an invalid invite id registers nothing",
			device: Device{InviteID: "not-an-object-id", Label: "laptop", OSUser: "ana"},
		},
		{
			name:    "an os user found by the exact match records the access",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop", OSUser: "ana"},
			replies: []deviceReply{{id: "d1"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(nil)},
			},
			wantID: "d1",
		},
		{
			name:    "a new os user adopts the invite's device of that name without one",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop", OSUser: "ana"},
			replies: []deviceReply{{}, {id: "d2"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(nil)},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldOSUser: "ana"})},
			},
			wantID: "d2",
		},
		{
			name:    "the machine's device under another invite is moved into this one",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop", OSUser: "ana"},
			replies: []deviceReply{{}, {}, {id: "d3"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(nil)},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldOSUser: "ana"})},
				{filter: anyInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(bson.M{fieldInviteID: inviteOID})},
			},
			wantID: "d3",
		},
		{
			name:    "a device under another invite from before os users is adopted with both filled in",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop", OSUser: "ana"},
			replies: []deviceReply{{}, {}, {}, {id: "d4"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(nil)},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldOSUser: "ana"})},
				{filter: anyInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(bson.M{fieldInviteID: inviteOID})},
				{filter: anyInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldInviteID: inviteOID, fieldOSUser: "ana"})},
			},
			wantID: "d4",
		},
		{
			name:    "a lease without an os user adopts the same name's device under another invite",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop"},
			replies: []deviceReply{{}, {id: "d5"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(nil)},
				{filter: anyInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldInviteID: inviteOID})},
			},
			wantID: "d5",
		},
		{
			name:    "inserts when no key finds a device",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop", OSUser: "ana"},
			replies: []deviceReply{{}, {}, {}, {}, {id: "d6"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(nil)},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldOSUser: "ana"})},
				{filter: anyInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(bson.M{fieldInviteID: inviteOID})},
				{filter: anyInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldInviteID: inviteOID, fieldOSUser: "ana"})},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: insert(inviteOID, "laptop", "ana"), upsert: true},
			},
			wantID: "d6",
		},
		{
			name:    "a concurrent insert is re-read once",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop"},
			replies: []deviceReply{{}, {}, {err: errDuplicateDevice}, {id: "d7"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(nil)},
				{filter: anyInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldInviteID: inviteOID})},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: insert(inviteOID, "laptop", nil), upsert: true},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(nil)},
			},
			wantID: "d7",
		},
		{
			name:    "a duplicate from adoption is re-read once too",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop", OSUser: "ana"},
			replies: []deviceReply{{}, {err: errDuplicateDevice}, {id: "d8"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(nil)},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldOSUser: "ana"})},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(nil)},
			},
			wantID: "d8",
		},
		{
			name:    "a re-read that finds nothing fails",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop"},
			replies: []deviceReply{{}, {}, {err: errDuplicateDevice}, {}, {id: "never asked"}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(nil)},
				{filter: anyInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(bson.M{fieldInviteID: inviteOID})},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: insert(inviteOID, "laptop", nil), upsert: true},
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: nil}), update: access(nil)},
			},
			wantErr: true,
		},
		{
			name:    "a store error stops the registration",
			device:  Device{InviteID: deviceTestInvite, Label: "laptop", OSUser: "ana"},
			replies: []deviceReply{{err: errStore}},
			want: []deviceQuery{
				{filter: withInvite(bson.M{fieldName: "laptop", fieldOSUser: "ana"}), update: access(nil)},
			},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.device.Origin = "198.51.100.7"
			c.device.LastSeenAt = deviceTestTime
			devices := &fakeDevices{replies: c.replies}

			id, err := registerDevice(context.Background(), c.device, devices.update)
			if (err != nil) != c.wantErr || id != c.wantID {
				t.Errorf("registerDevice() = %q, %v; want %q, error %v", id, err, c.wantID, c.wantErr)
			}
			if !reflect.DeepEqual(devices.queries, c.want) {
				t.Errorf("queries:\n got %+v\nwant %+v", devices.queries, c.want)
			}
		})
	}
}
