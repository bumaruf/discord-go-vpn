package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const inviteToken = "vpn_invite-token-for-tests"

// fakeInvites holds one active invite for inviteToken and counts lookups; any other store
// call panics on the nil embedded store.
type fakeInvites struct {
	store.Store
	maxLeases *int
	lookups   int
}

func (f *fakeInvites) FindInvite(_ context.Context, hash string) (*store.InviteRecord, error) {
	f.lookups++
	if hash != HashToken(inviteToken) {
		return nil, nil
	}
	return &store.InviteRecord{ID: bson.NewObjectID(), Status: store.InviteActive, MaxLeases: f.maxLeases}, nil
}

func TestIdentifyWithoutDatabase(t *testing.T) {
	a := NewAuthenticator(nil, time.Now)

	if invite, ok, err := a.Identify(context.Background(), inviteToken); ok || invite.ID != "" || err != nil {
		t.Errorf("invite: %+v, ok %v, err %v; want a refusal", invite, ok, err)
	}
	if _, cached := a.cached(HashToken(inviteToken)); cached {
		t.Error("the refusal without a database was cached")
	}
}

func TestIdentifyInvite(t *testing.T) {
	invites := &fakeInvites{}
	a := NewAuthenticator(invites, time.Now)

	if invite, ok, err := a.Identify(context.Background(), inviteToken); !ok || invite.ID == "" || invite.MaxLeases != 0 || err != nil {
		t.Errorf("invite: %+v, ok %v, err %v; want accepted with its id and no cap", invite, ok, err)
	}
	if invites.lookups != 1 {
		t.Errorf("lookups = %d, want 1", invites.lookups)
	}
}

func TestIdentifyReturnsTheLeaseCap(t *testing.T) {
	for _, c := range []struct {
		stored int
		want   int
	}{{4, 4}, {0, 0}, {-1, 0}} {
		stored := c.stored
		a := NewAuthenticator(&fakeInvites{maxLeases: &stored}, time.Now)

		invite, ok, err := a.Identify(context.Background(), inviteToken)
		if !ok || err != nil || invite.MaxLeases != c.want {
			t.Errorf("stored cap %d: %+v, ok %v, err %v; want MaxLeases %d", c.stored, invite, ok, err, c.want)
		}
	}
}

func TestIdentifyRefusesMalformedTokensWithoutLookup(t *testing.T) {
	invites := &fakeInvites{}
	a := NewAuthenticator(invites, time.Now)

	for _, token := range []string{"", "vpn_", "abc_defgh", "vpn_UPPER", "vpn_with space", "vpn_" + strings.Repeat("a", maxTokenLength)} {
		if invite, ok, err := a.Identify(context.Background(), token); ok || invite.ID != "" || err != nil {
			t.Errorf("%q: %+v, ok %v, err %v; want a refusal", token, invite, ok, err)
		}
	}
	if invites.lookups != 0 {
		t.Errorf("lookups = %d, want none", invites.lookups)
	}
	if len(a.cache) != 0 {
		t.Errorf("cache holds %d entries, want none", len(a.cache))
	}
}

func TestCacheDropsExpiredEntries(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	a := NewAuthenticator(&fakeInvites{}, func() time.Time { return now })

	for _, token := range []string{"vpn_made-up-a", "vpn_made-up-b", "vpn_made-up-c"} {
		_, _, _ = a.Identify(context.Background(), token)
	}

	now = now.Add(cacheTTL + time.Second)
	_, _, _ = a.Identify(context.Background(), "vpn_made-up-d")

	if len(a.cache) != 1 {
		t.Errorf("cache holds %d entries, want only the new one", len(a.cache))
	}
}
