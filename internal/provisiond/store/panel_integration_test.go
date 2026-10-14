package store

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestPanelDataWithDatabase runs the panel's queries against a real Mongo, with documents
// written the way the collector and the old panel wrote them:
//
//	MONGODB_INTEGRATION=1 MONGODB_URI=mongodb://localhost:27017 go test ./internal/provisiond/store/ -run TestPanelDataWithDatabase
//
// It drops the database vpn_panel_test on whatever cluster the URI points to.
func TestPanelDataWithDatabase(t *testing.T) {
	if os.Getenv("MONGODB_INTEGRATION") != "1" {
		t.Skip("set MONGODB_INTEGRATION=1 and MONGODB_URI to run the panel queries against a real database")
	}
	if os.Getenv("MONGODB_URI") == "" {
		t.Skip("MONGODB_INTEGRATION=1 needs MONGODB_URI")
	}
	t.Setenv("MONGODB_DB", "vpn_panel_test")

	ctx := context.Background()
	m, err := Open(ctx, config.Load())
	if err != nil || m == nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer m.Close(ctx)
	if err := m.db.Drop(ctx); err != nil {
		t.Fatalf("dropping the database: %v", err)
	}

	day := func(d int) time.Time { return time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC) }
	mustInsert := func(collection string, docs ...any) {
		t.Helper()
		if _, err := m.db.Collection(collection).InsertMany(ctx, docs); err != nil {
			t.Fatalf("seeding %s: %v", collection, err)
		}
	}

	older, newer := bson.NewObjectID(), bson.NewObjectID()
	mustInsert(invitesCollection,
		// Stored before kinds existed: no kind, no cap.
		bson.M{"_id": older, fieldLabel: "antigo", fieldTokenHash: "a", fieldPrefix: "vpn_aa", fieldStatus: InviteRevoked, fieldCreatedAt: day(1)},
		bson.M{"_id": newer, fieldLabel: "Família", fieldKind: InviteGlobal, fieldTokenHash: "b", fieldPrefix: "vpn_bb", fieldStatus: InviteActive, fieldCreatedAt: day(2), fieldMaxLeases: 5},
	)
	mustInsert(devicesCollection,
		bson.M{fieldInviteID: newer, fieldName: "PC-ANA", fieldOSUser: "ana", fieldLastIP: "203.0.113.7", fieldCreatedAt: day(1), fieldLastSeenAt: day(3), fieldAccessCount: 4},
		bson.M{fieldInviteID: newer, fieldName: "CELULAR", fieldCreatedAt: day(1), fieldLastSeenAt: day(2), fieldAccessCount: 1},
		// Left behind by the removed global token: no invite, no os user.
		bson.M{fieldInviteID: nil, fieldName: "", fieldCreatedAt: day(1), fieldLastSeenAt: day(1), fieldAccessCount: 1},
	)

	t.Run("invites, newest first, with kind default and device counts", func(t *testing.T) {
		invites, err := m.ListInvites(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		five := 5
		want := []InviteSummary{
			{ID: newer.Hex(), Label: "Família", Kind: InviteGlobal, Prefix: "vpn_bb", Status: InviteActive, CreatedAt: day(2), MaxLeases: &five, DeviceCount: 2},
			{ID: older.Hex(), Label: "antigo", Kind: InviteIndividual, Prefix: "vpn_aa", Status: InviteRevoked, CreatedAt: day(1)},
		}
		if !reflect.DeepEqual(invites, want) {
			t.Errorf("invites = %+v\nwant %+v", invites, want)
		}
	})

	t.Run("invite labels", func(t *testing.T) {
		labels, err := m.InviteLabels(ctx, 200)
		if err != nil {
			t.Fatal(err)
		}
		if want := map[string]string{older.Hex(): "antigo", newer.Hex(): "Família"}; !reflect.DeepEqual(labels, want) {
			t.Errorf("labels = %v, want %v", labels, want)
		}
	})

	t.Run("devices, most recently seen first", func(t *testing.T) {
		devices, err := m.ListDevices(ctx, 500)
		if err != nil {
			t.Fatal(err)
		}
		if len(devices) != 3 {
			t.Fatalf("devices = %d, want 3", len(devices))
		}
		first := devices[0]
		if first.Name != "PC-ANA" || first.InviteID != newer.Hex() || first.OSUser != "ana" || first.LastIP != "203.0.113.7" ||
			!first.LastSeenAt.Equal(day(3)) || first.AccessCount != 4 {
			t.Errorf("first device = %+v", first)
		}
		if last := devices[2]; last.InviteID != "" || last.OSUser != "" {
			t.Errorf("device without invite = %+v", last)
		}
	})

	t.Run("usage by day, of the server and of one device", func(t *testing.T) {
		// The collector writes these with $inc from uint64 counters.
		for _, u := range []DailyUsage{
			{Identity: "dev1", Day: "2026-08-31", RxBytes: 999, TxBytes: 999},
			{Identity: "dev1", Day: "2026-09-01", RxBytes: 100, TxBytes: 10},
			{Identity: "dev2", Day: "2026-09-01", RxBytes: 50, TxBytes: 5},
			{Identity: "dev2", Day: "2026-09-02", RxBytes: 7, TxBytes: 1},
		} {
			if err := m.AddDailyUsage(ctx, u); err != nil {
				t.Fatal(err)
			}
		}

		server, err := m.UsageByDay(ctx, "2026-09-01", "")
		if err != nil {
			t.Fatal(err)
		}
		if want := []DayUsage{{"2026-09-01", 150, 15}, {"2026-09-02", 7, 1}}; !reflect.DeepEqual(server, want) {
			t.Errorf("server usage = %v, want %v", server, want)
		}

		device, err := m.UsageByDay(ctx, "2026-09-01", "dev2")
		if err != nil {
			t.Fatal(err)
		}
		if want := []DayUsage{{"2026-09-01", 50, 5}, {"2026-09-02", 7, 1}}; !reflect.DeepEqual(device, want) {
			t.Errorf("device usage = %v, want %v", device, want)
		}
	})

	t.Run("insert, cap and delete an invite", func(t *testing.T) {
		three := 3
		err := m.InsertInvite(ctx, NewInvite{Label: "novo", Kind: InviteIndividual, TokenHash: "hash-novo", Prefix: "vpn_cc", CreatedBy: "admin", CreatedAt: day(4), MaxLeases: &three})
		if err != nil {
			t.Fatal(err)
		}
		record, err := m.FindInvite(ctx, "hash-novo")
		if err != nil || record == nil || record.Status != InviteActive || record.MaxLeases == nil || *record.MaxLeases != 3 {
			t.Fatalf("FindInvite after insert = %+v, %v", record, err)
		}
		id := record.ID.Hex()

		if ok, err := m.SetInviteMaxLeases(ctx, id, nil); err != nil || !ok {
			t.Fatalf("removing the cap = %v, %v", ok, err)
		}
		if record, _ := m.FindInvite(ctx, "hash-novo"); record.MaxLeases != nil {
			t.Errorf("cap after removal = %d", *record.MaxLeases)
		}

		if ok, err := m.DeleteInvite(ctx, id); err != nil || !ok {
			t.Fatalf("deleting = %v, %v", ok, err)
		}
		if ok, err := m.DeleteInvite(ctx, id); err != nil || ok {
			t.Errorf("deleting again = %v, %v, want false", ok, err)
		}
		if ok, err := m.SetInviteMaxLeases(ctx, "not-an-id", nil); err != nil || ok {
			t.Errorf("capping a malformed id = %v, %v, want false", ok, err)
		}
	})

	t.Run("audit entry", func(t *testing.T) {
		if err := m.InsertAudit(ctx, AuditEntry{ActorID: "admin", Action: "disconnect", Target: "lease1", OK: false, Detail: "timeout", At: day(5)}); err != nil {
			t.Fatal(err)
		}
		var doc bson.M
		if err := m.db.Collection(auditLogCollection).FindOne(ctx, bson.M{}).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		if doc[fieldResult] != auditError || doc[fieldAction] != "disconnect" || doc[fieldActorID] != "admin" {
			t.Errorf("audit doc = %v", doc)
		}
	})
}
