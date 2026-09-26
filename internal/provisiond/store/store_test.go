package store

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSessionUpsert(t *testing.T) {
	inviteOID, err := bson.ObjectIDFromHex(deviceTestInvite)
	if err != nil {
		t.Fatal(err)
	}
	started := deviceTestTime.Add(-time.Hour)
	handshake := deviceTestTime.Add(-10 * time.Second)

	session := Session{
		LeaseID:   "lease",
		DeviceID:  "65f1a2b3c4d5e6f7a8b9c0d2",
		Label:     "laptop",
		PublicKey: "key",
		IP:        "10.9.0.10",
		Origin:    "198.51.x.x",
		RxBytes:   100,
		TxBytes:   200,
		UpdatedAt: deviceTestTime,
		StartedAt: started,
	}
	doc := func(inviteID any, status string) bson.M {
		return bson.M{
			fieldInviteID:  inviteID,
			fieldDeviceID:  "65f1a2b3c4d5e6f7a8b9c0d2",
			fieldLabel:     "laptop",
			fieldPublicKey: "key",
			fieldIP:        "10.9.0.10",
			fieldOriginIP:  "198.51.x.x",
			fieldRxBytes:   uint64(100),
			fieldTxBytes:   uint64(200),
			fieldStatus:    status,
			fieldUpdatedAt: deviceTestTime,
		}
	}

	withInvite := session
	withInvite.InviteID = deviceTestInvite
	withInvite.Online = true
	withInvite.LastHandshake = handshake
	withInviteDoc := doc(inviteOID, sessionOnline)
	withInviteDoc[fieldLastHandshake] = handshake

	cases := []struct {
		name    string
		session Session
		want    bson.M
	}{
		{"an invite is stored as its ObjectId", withInvite, withInviteDoc},
		{"no invite is stored as null, without a handshake", session, doc(nil, sessionOffline)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			filter, update := sessionUpsert(c.session)
			if want := (bson.M{"_id": "lease"}); !reflect.DeepEqual(filter, want) {
				t.Errorf("filter = %v, want %v", filter, want)
			}
			want := bson.M{"$set": c.want, "$setOnInsert": bson.M{fieldStartedAt: started}}
			if !reflect.DeepEqual(update, want) {
				t.Errorf("update:\n got %v\nwant %v", update, want)
			}
		})
	}
}
