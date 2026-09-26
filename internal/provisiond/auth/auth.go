package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
)

// cacheTTL spares Atlas a lookup on every /lease and /renew, at the cost of an invite's
// status being up to this stale unless Invalidate drops it.
const cacheTTL = 60 * time.Second

const (
	invitePrefix = "vpn_"
	// maxTokenLength is well above the length of the codes panel/lib/token.ts generates.
	maxTokenLength = 64
)

// Invite is what a lease needs to know about the invite its token belongs to.
type Invite struct {
	ID string
	// MaxLeases caps the invite's simultaneous leases; 0 means no cap.
	MaxLeases int
}

type cachedInvite struct {
	invite    Invite
	valid     bool
	expiresAt time.Time
}

type Authenticator struct {
	store store.Store // nil without a database
	now   func() time.Time

	mu      sync.Mutex
	cache   map[string]cachedInvite // token hash -> invite
	sweptAt time.Time
}

func NewAuthenticator(st store.Store, now func() time.Time) *Authenticator {
	return &Authenticator{
		store: st,
		cache: make(map[string]cachedInvite),
		now:   now,
	}
}

// Identify returns the invite the token belongs to, individual or global alike; ok is false
// for anything but an active invite. err means the database couldn't answer: not a refusal,
// and not cached.
func (a *Authenticator) Identify(ctx context.Context, token string) (invite Invite, ok bool, err error) {
	// Anyone can call the lease routes, so a token that can't be an invite costs no lookup
	// and takes no room in the cache.
	if !wellFormed(token) {
		return Invite{}, false, nil
	}

	hash := HashToken(token)

	if c, found := a.cached(hash); found {
		return c.invite, c.valid, nil
	}

	if a.store == nil {
		return Invite{}, false, nil
	}

	c, err := a.lookup(ctx, hash)
	if err != nil {
		return Invite{}, false, err
	}

	a.remember(hash, c)
	return c.invite, c.valid, nil
}

func wellFormed(token string) bool {
	code, found := strings.CutPrefix(token, invitePrefix)
	if !found || code == "" || len(token) > maxTokenLength {
		return false
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func (a *Authenticator) cached(hash string) (cachedInvite, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	c, found := a.cache[hash]
	if !found || a.now().After(c.expiresAt) {
		return cachedInvite{}, false
	}
	return c, true
}

// remember caches c, sweeping the expired entries at most once per TTL, so made-up tokens
// can't grow the cache beyond what one TTL of requests adds.
func (a *Authenticator) remember(hash string, c cachedInvite) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.now()
	if now.Sub(a.sweptAt) >= cacheTTL {
		for h, entry := range a.cache {
			if now.After(entry.expiresAt) {
				delete(a.cache, h)
			}
		}
		a.sweptAt = now
	}
	a.cache[hash] = c
}

func (a *Authenticator) lookup(ctx context.Context, hash string) (cachedInvite, error) {
	now := a.now()
	result := cachedInvite{expiresAt: now.Add(cacheTTL)}

	doc, err := a.store.FindInvite(ctx, hash)
	if err != nil {
		return cachedInvite{}, err
	}
	if doc == nil {
		// A refusal is cached too, so a client retrying a made-up token costs one lookup a minute.
		return result, nil
	}

	// Kept on a refusal too, so the log can say which invite was refused.
	result.invite = Invite{ID: doc.ID.Hex()}
	if doc.MaxLeases != nil && *doc.MaxLeases > 0 {
		result.invite.MaxLeases = *doc.MaxLeases
	}

	if doc.Status != store.InviteActive {
		return result, nil
	}
	if doc.ExpiresAt != nil && now.After(*doc.ExpiresAt) {
		return result, nil
	}

	result.valid = true
	return result, nil
}

// Invalidate drops the invite's cached tokens, so its new status applies to the next request.
func (a *Authenticator) Invalidate(inviteID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for hash, c := range a.cache {
		if c.invite.ID == inviteID {
			delete(a.cache, hash)
		}
	}
}

// HashToken is what the database stores instead of the invite code, computed the same way
// in panel/lib/token.ts. Plain SHA-256 is enough: the code is random, not a password, and
// this runs on every lease request.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
