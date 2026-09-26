package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestAuthorizationWithDatabase runs invite authorization against a real Mongo, with the
// invites written the way the panel writes them:
//
//	MONGODB_INTEGRATION=1 MONGODB_URI=mongodb://localhost:27017 go test ./internal/provisiond/auth/ -run TestAuthorizationWithDatabase
//
// MONGODB_URI alone doesn't enable it: make test runs this package, and the
// test drops a collection on whatever cluster the URI points to.
func TestAuthorizationWithDatabase(t *testing.T) {
	if os.Getenv("MONGODB_INTEGRATION") != "1" {
		t.Skip("set MONGODB_INTEGRATION=1 and MONGODB_URI to run authorization against a real database")
	}
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("MONGODB_INTEGRATION=1 needs MONGODB_URI")
	}
	t.Setenv("MONGODB_DB", "vpn_auth_test")

	ctx := context.Background()
	cfg := config.Load()
	db, err := store.Open(ctx, cfg)
	if err != nil || db == nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer db.Close(ctx)

	// The invites are written through a client of the test's own, as the panel writes them.
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connecting to write the invites: %v", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			t.Errorf("disconnecting the invites client: %v", err)
		}
	}()

	invites := client.Database(cfg.MongoDatabaseName()).Collection("invites")
	if err := invites.Drop(ctx); err != nil {
		t.Fatalf("dropping the collection: %v", err)
	}

	const activeCode = "vpn_abcdefgh-ijkmnpqr-stuvwxyz"
	const revokedCode = "vpn_zyxwvuts-rqpnmkji-hgfedcba"

	active := bson.NewObjectID()
	revoked := bson.NewObjectID()

	_, err = invites.InsertMany(ctx, []any{
		bson.M{"_id": active, "label": "group A", "tokenHash": HashToken(activeCode), "status": "active", "createdAt": time.Now()},
		bson.M{"_id": revoked, "label": "former group", "tokenHash": HashToken(revokedCode), "status": "revoked", "createdAt": time.Now()},
	})
	if err != nil {
		t.Fatalf("inserting invites: %v", err)
	}

	a := NewAuthenticator(db, time.Now)
	identify := func(t *testing.T, token string) (string, bool) {
		t.Helper()
		invite, ok, err := a.Identify(ctx, token)
		if err != nil {
			t.Fatalf("invite lookup: %v", err)
		}
		return invite.ID, ok
	}

	t.Run("an active invite gets in with its id", func(t *testing.T) {
		id, ok := identify(t, activeCode)
		if !ok {
			t.Fatal("refused an active invite")
		}
		if id != active.Hex() {
			t.Errorf("invite = %q, want %q", id, active.Hex())
		}
	})

	t.Run("a revoked invite doesn't get in", func(t *testing.T) {
		a.Invalidate(revoked.Hex()) // drops what the previous subtest cached
		if _, ok := identify(t, revokedCode); ok {
			t.Error("accepted a revoked invite")
		}
	})

	t.Run("a made-up code doesn't get in", func(t *testing.T) {
		if _, ok := identify(t, "vpn_notfound-notfound-notfound"); ok {
			t.Error("accepted a code that doesn't exist")
		}
	})

	t.Run("revoking during a session takes effect at once", func(t *testing.T) {
		if _, ok := identify(t, activeCode); !ok {
			t.Fatal("the invite should be valid before revoking it")
		}

		_, err := invites.UpdateOne(ctx, bson.M{"_id": active}, bson.M{"$set": bson.M{"status": "revoked"}})
		if err != nil {
			t.Fatalf("revoking: %v", err)
		}
		a.Invalidate(active.Hex())

		if _, ok := identify(t, activeCode); ok {
			t.Error("still accepted after being revoked and invalidated")
		}
	})
}
