package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/lease"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/live"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/metrics"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard/wireguardtest"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// testToken is the invite every test server accepts, with testTokenInviteID.
const (
	testToken         = "vpn_default-token-for-tests"
	testTokenInviteID = "65f1a2b3c4d5e6f7a8b9c0d0"
)

type fakeClock struct {
	current time.Time
}

func (c *fakeClock) now() time.Time { return c.current }

func (c *fakeClock) advance(d time.Duration) { c.current = c.current.Add(d) }

type testServer struct {
	*server
	wg      *wireguardtest.Fake
	clock   *fakeClock
	invites *fakeInvites
}

// newTestServer builds a server with no database but fake invite lookups, fake wg and a
// fixed clock; only testToken and the tokens given to addInvite are accepted.
func newTestServer(t *testing.T) *testServer {
	t.Helper()
	return newLimitedTestServer(t, 50)
}

// newLimitedTestServer is newTestServer with a limit of maxActive leases.
func newLimitedTestServer(t *testing.T, maxActive int) *testServer {
	t.Helper()
	wg := wireguardtest.NewFake()
	clock := &fakeClock{current: time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)}
	invites := &fakeInvites{records: make(map[string]store.InviteRecord)}
	s := &server{
		registry:  lease.NewRegistry(wg, config.DefaultWGSubnet, clock.now, maxActive),
		wg:        wg,
		now:       clock.now,
		random:    rand.Reader,
		endpoint:  "203.0.113.1:51820",
		dns:       "1.1.1.1",
		publicKey: "server-public-key",
		auth:      auth.NewAuthenticator(invites, clock.now),
	}
	s.hub = live.NewHub()
	s.collector = metrics.NewCollector(wg, s.registry, nil, clock.now, s.hub.Publish)
	ts := &testServer{server: s, wg: wg, clock: clock, invites: invites}
	ts.addInvite(testToken, testTokenInviteID)
	return ts
}

// defaultTTL names lease.TTL where the tests' lease variables would shadow the package.
const defaultTTL = lease.TTL

// leases returns a copy of the registry's leases.
func (ts *testServer) leases() map[string]*lease.Lease {
	leases := make(map[string]*lease.Lease)
	for id, l := range ts.registry.Leases() {
		leases[id] = &l
	}
	return leases
}

func (ts *testServer) serve(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	ts.routes().ServeHTTP(rec, req)
	return rec
}

func (ts *testServer) post(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return ts.serve(postRequest(t, path, body))
}

func postRequest(t *testing.T, path string, body any) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding request body: %v", err)
	}
	return httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
}

// addInvite makes token a valid invite with inviteID, as the database would answer.
func (ts *testServer) addInvite(token, inviteID string) {
	id, err := bson.ObjectIDFromHex(inviteID)
	if err != nil {
		panic(err)
	}
	ts.invites.records[auth.HashToken(token)] = store.InviteRecord{ID: id, Status: store.InviteActive}
}

func (ts *testServer) acquire(t *testing.T, publicKey string) acquireResponse {
	t.Helper()
	return ts.acquireWith(t, testToken, publicKey)
}

func (ts *testServer) acquireWith(t *testing.T, token, publicKey string) acquireResponse {
	t.Helper()
	rec := ts.post(t, "/lease", acquireRequest{Token: token, PubKey: publicKey})
	if rec.Code != http.StatusOK {
		t.Fatalf("/lease status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var resp acquireResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding /lease response: %v", err)
	}
	return resp
}

// fillAddressRange stores an active lease on every tunnel address.
func (ts *testServer) fillAddressRange() {
	for i := lease.FirstIP; i <= lease.LastIP; i++ {
		ts.registry.Grant(fmt.Sprintf("lease-%d", i), lease.Lease{IP: fmt.Sprintf("10.9.0.%d", i)})
	}
}

// assertNoReservedAddresses leases up to the limit through /lease, since an address left
// reserved counts toward it and would refuse the last lease. It leaves the server full.
func (ts *testServer) assertNoReservedAddresses(t *testing.T) {
	t.Helper()
	free := ts.registry.MaxActive() - len(ts.leases())
	for i := range free {
		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(byte(200 + i))})
		if rec.Code != http.StatusOK {
			t.Fatalf("lease %d of the %d below the limit: status %d, want 200 (an address is still reserved)", i+1, free, rec.Code)
		}
	}
}

// testPublicKey returns a valid WireGuard public key, distinct per n.
func testPublicKey(n byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{n}, 32))
}

// captureLog redirects the standard logger for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, want, rec.Body.String())
	}
}

func TestLeaseAcquire(t *testing.T) {
	t.Run("grants a peer and a lease", func(t *testing.T) {
		ts := newTestServer(t)
		key := testPublicKey(1)

		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: key, Label: " laptop\n", OSUser: " ana\n"})
		assertStatus(t, rec, http.StatusOK)

		var resp acquireResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decoding response: %v", err)
		}
		want := acquireResponse{ID: resp.ID, Address: "10.9.0.10/32", DNS: "1.1.1.1", ServerPub: "server-public-key", Endpoint: "203.0.113.1:51820", leaseTTL: leaseTTL{Seconds: 180}}
		if resp != want || resp.ID == "" {
			t.Errorf("response = %+v, want %+v with a non-empty id", resp, want)
		}
		if ip := ts.wg.Peers[key]; ip != "10.9.0.10" {
			t.Errorf("wg peer IP = %q, want 10.9.0.10", ip)
		}

		l := ts.leases()[resp.ID]
		if l == nil {
			t.Fatal("lease not stored")
		}
		if !l.ExpiresAt.Equal(ts.clock.now().Add(defaultTTL)) || !l.Since.Equal(ts.clock.now()) {
			t.Errorf("lease times = since %v, expires %v; want now and now+TTL", l.Since, l.ExpiresAt)
		}
		if l.Label != "laptop" || l.OSUser != "ana" {
			t.Errorf("label %q, os user %q; want the cleaned laptop and ana", l.Label, l.OSUser)
		}
	})

	t.Run("401 for an unknown token", func(t *testing.T) {
		ts := newTestServer(t)
		rec := ts.post(t, "/lease", acquireRequest{Token: "not-a-valid-token", PubKey: testPublicKey(1)})
		assertStatus(t, rec, http.StatusUnauthorized)
		if len(ts.wg.Peers) != 0 {
			t.Error("a peer was added for a rejected token")
		}
	})

	t.Run("400 for an invalid public key", func(t *testing.T) {
		ts := newTestServer(t)
		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: "short"})
		assertStatus(t, rec, http.StatusBadRequest)
	})

	t.Run("400 for a malformed body", func(t *testing.T) {
		ts := newTestServer(t)
		rec := ts.serve(httptest.NewRequest(http.MethodPost, "/lease", strings.NewReader("{")))
		assertStatus(t, rec, http.StatusBadRequest)
	})

	t.Run("405 for GET", func(t *testing.T) {
		ts := newTestServer(t)
		rec := ts.serve(httptest.NewRequest(http.MethodGet, "/lease", nil))
		assertStatus(t, rec, http.StatusMethodNotAllowed)
	})

	t.Run("429 at the active lease limit", func(t *testing.T) {
		ts := newLimitedTestServer(t, 1)
		ts.acquire(t, testPublicKey(1))

		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(2)})
		assertStatus(t, rec, http.StatusTooManyRequests)
		if _, added := ts.wg.Peers[testPublicKey(2)]; added {
			t.Error("a peer was added past the limit")
		}
	})

	t.Run("expired leases free their slot before the limit is checked", func(t *testing.T) {
		ts := newLimitedTestServer(t, 1)
		ts.acquire(t, testPublicKey(1))
		ts.clock.advance(defaultTTL + time.Second)

		ts.acquire(t, testPublicKey(2))
		if _, kept := ts.wg.Peers[testPublicKey(1)]; kept {
			t.Error("the expired peer is still on the interface")
		}
	})

	t.Run("503 when the address range is exhausted, with the cause only in the log", func(t *testing.T) {
		ts := newLimitedTestServer(t, 1000)
		ts.fillAddressRange()
		logged := captureLog(t)

		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(1)})
		assertStatus(t, rec, http.StatusServiceUnavailable)
		if got := strings.TrimSpace(rec.Body.String()); got != "o servidor está sem vagas no momento; tente de novo em alguns minutos" {
			t.Errorf("body = %q, want the fixed capacity message", got)
		}
		if !strings.Contains(logged.String(), "no free address in the range") {
			t.Errorf("log = %q, want freeIP's error", logged.String())
		}
	})

	t.Run("500 when wg fails, without storing the lease, keeping its address or leaking the error", func(t *testing.T) {
		ts := newTestServer(t)
		ts.wg.AddErr = errors.New("wg: exit status 1")

		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(1)})
		assertStatus(t, rec, http.StatusInternalServerError)
		if strings.Contains(rec.Body.String(), "exit status") {
			t.Errorf("body = %q, want no internal error text", rec.Body.String())
		}
		if len(ts.leases()) != 0 {
			t.Errorf("leases %v; want none after wg failed", ts.leases())
		}

		ts.wg.AddErr = nil
		if l := ts.acquire(t, testPublicKey(2)); l.Address != "10.9.0.10/32" {
			t.Errorf("next address = %q, want 10.9.0.10/32 back", l.Address)
		}
		ts.assertNoReservedAddresses(t)
	})

	t.Run("500 when the random source fails, without a lease, reservation, peer or device", func(t *testing.T) {
		ts := newTestServer(t)
		ts.random = iotest.ErrReader(errors.New("entropy source unavailable"))
		devices := ts.recordDevices()

		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(1)})
		assertStatus(t, rec, http.StatusInternalServerError)
		if strings.Contains(rec.Body.String(), "entropy") {
			t.Errorf("body = %q, want no internal error text", rec.Body.String())
		}
		if len(ts.leases()) != 0 || len(ts.wg.Peers) != 0 || len(*devices) != 0 {
			t.Errorf("leases %v, peers %v, devices %q; want none", ts.leases(), ts.wg.Peers, *devices)
		}

		ts.random = rand.Reader
		ts.assertNoReservedAddresses(t)
	})
}

// deviceStore fakes device registration; any other store call panics on the nil embedded store.
type deviceStore struct {
	store.Store
	register func(d store.Device) string
}

func (f deviceStore) RegisterDevice(_ context.Context, d store.Device) (string, error) {
	return f.register(d), nil
}

// recordDevices replaces device registration with a fake that records each label and
// returns "device-<label>" as the id.
func (ts *testServer) recordDevices() *[]string {
	var labels []string
	ts.store = deviceStore{register: func(d store.Device) string {
		labels = append(labels, d.Label)
		return "device-" + d.Label
	}}
	return &labels
}

func TestLeaseDeviceRegistration(t *testing.T) {
	t.Run("a granted lease registers its device once", func(t *testing.T) {
		ts := newTestServer(t)
		labels := ts.recordDevices()

		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(1), Label: "laptop"})
		assertStatus(t, rec, http.StatusOK)

		if !slices.Equal(*labels, []string{"laptop"}) {
			t.Errorf("registered devices = %q, want [laptop]", *labels)
		}
		var resp acquireResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decoding response: %v", err)
		}
		if l := ts.leases()[resp.ID]; l == nil || l.DeviceID != "device-laptop" {
			t.Errorf("lease = %+v, want DeviceID device-laptop", l)
		}
	})

	t.Run("the device gets the cleaned os user", func(t *testing.T) {
		ts := newTestServer(t)
		var registered store.Device
		ts.store = deviceStore{register: func(d store.Device) string {
			registered = d
			return "device"
		}}

		rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(1), Label: "laptop", OSUser: "ana\x00 "})
		assertStatus(t, rec, http.StatusOK)
		if registered.Label != "laptop" || registered.OSUser != "ana" {
			t.Errorf("registered label %q, os user %q; want laptop and ana", registered.Label, registered.OSUser)
		}
	})

	rejections := []struct {
		name   string
		limit  int
		setup  func(ts *testServer)
		status int
	}{
		{"429 at the active lease limit", 1, func(ts *testServer) {
			ts.registry.Grant("existing", lease.Lease{IP: "10.9.0.10"})
		}, http.StatusTooManyRequests},
		{"503 when the address range is exhausted", 1000, func(ts *testServer) {
			ts.fillAddressRange()
		}, http.StatusServiceUnavailable},
		{"500 when wg fails", 50, func(ts *testServer) {
			ts.wg.AddErr = errors.New("wg: exit status 1")
		}, http.StatusInternalServerError},
	}
	for _, c := range rejections {
		t.Run(c.name+" registers no device", func(t *testing.T) {
			ts := newLimitedTestServer(t, c.limit)
			c.setup(ts)
			labels := ts.recordDevices()

			rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(1), Label: "laptop"})
			assertStatus(t, rec, c.status)
			if len(*labels) != 0 {
				t.Errorf("registered devices = %q, want none", *labels)
			}
		})
	}
}

// Device registration runs outside the registry's lock, so these make a second /lease while the first
// one's device is being registered.
func TestLeaseHeldDuringDeviceRegistration(t *testing.T) {
	acquireDuringRegistration := func(t *testing.T, ts *testServer) (first acquireResponse, second *httptest.ResponseRecorder) {
		t.Helper()
		started := false
		ts.store = deviceStore{register: func(store.Device) string {
			if !started {
				started = true
				second = ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(2)})
			}
			return ""
		}}
		first = ts.acquire(t, testPublicKey(1))
		return first, second
	}

	t.Run("its slot counts toward the limit", func(t *testing.T) {
		ts := newLimitedTestServer(t, 1)

		first, second := acquireDuringRegistration(t, ts)
		assertStatus(t, second, http.StatusTooManyRequests)
		if first.Address != "10.9.0.10/32" {
			t.Errorf("first address = %q, want 10.9.0.10/32", first.Address)
		}
	})

	t.Run("its address is not handed out again", func(t *testing.T) {
		// One slot above the two leases, for assertNoReservedAddresses.
		ts := newLimitedTestServer(t, 3)

		first, second := acquireDuringRegistration(t, ts)
		assertStatus(t, second, http.StatusOK)
		var resp acquireResponse
		if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decoding second response: %v", err)
		}
		if first.Address != "10.9.0.10/32" || resp.Address != "10.9.0.11/32" {
			t.Errorf("addresses = %q and %q, want 10.9.0.10/32 and 10.9.0.11/32", first.Address, resp.Address)
		}
		ts.assertNoReservedAddresses(t)
	})
}

func TestLeaseRenew(t *testing.T) {
	t.Run("pushes the expiry one TTL from now", func(t *testing.T) {
		ts := newTestServer(t)
		lease := ts.acquire(t, testPublicKey(1))
		ts.clock.advance(time.Minute)

		rec := ts.post(t, "/renew", idRequest{Token: testToken, ID: lease.ID})
		assertStatus(t, rec, http.StatusOK)
		if got, want := strings.TrimSpace(rec.Body.String()), `{"ttl_seconds":180}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
		if got := ts.leases()[lease.ID].ExpiresAt; !got.Equal(ts.clock.now().Add(defaultTTL)) {
			t.Errorf("expires at %v, want now+TTL", got)
		}
	})

	t.Run("401 for an unknown token", func(t *testing.T) {
		ts := newTestServer(t)
		lease := ts.acquire(t, testPublicKey(1))

		rec := ts.post(t, "/renew", idRequest{Token: "not-a-valid-token", ID: lease.ID})
		assertStatus(t, rec, http.StatusUnauthorized)
	})

	t.Run("410 for an unknown lease", func(t *testing.T) {
		ts := newTestServer(t)
		rec := ts.post(t, "/renew", idRequest{Token: testToken, ID: "missing"})
		assertStatus(t, rec, http.StatusGone)
	})

	t.Run("410 once the expired lease was reaped", func(t *testing.T) {
		ts := newTestServer(t)
		lease := ts.acquire(t, testPublicKey(1))
		ts.clock.advance(defaultTTL + time.Second)
		ts.registry.ExpireStale()

		rec := ts.post(t, "/renew", idRequest{Token: testToken, ID: lease.ID})
		assertStatus(t, rec, http.StatusGone)
	})

	// /renew doesn't call expireStale, so a lease past its expiry survives until the reaper runs.
	t.Run("renews an expired lease the reaper hasn't removed yet", func(t *testing.T) {
		ts := newTestServer(t)
		lease := ts.acquire(t, testPublicKey(1))
		ts.clock.advance(defaultTTL + time.Second)

		rec := ts.post(t, "/renew", idRequest{Token: testToken, ID: lease.ID})
		assertStatus(t, rec, http.StatusOK)
	})
}

func TestLeaseRelease(t *testing.T) {
	t.Run("removes the peer and the lease", func(t *testing.T) {
		ts := newTestServer(t)
		lease := ts.acquire(t, testPublicKey(1))

		rec := ts.post(t, "/release", idRequest{Token: testToken, ID: lease.ID})
		assertStatus(t, rec, http.StatusOK)
		if got := strings.TrimSpace(rec.Body.String()); got != `{"ok":true}` {
			t.Errorf("body = %s, want {\"ok\":true}", got)
		}
		if _, kept := ts.leases()[lease.ID]; kept {
			t.Error("lease still stored")
		}
		if _, kept := ts.wg.Peers[testPublicKey(1)]; kept {
			t.Error("peer still on the interface")
		}
	})

	t.Run("an unknown lease still answers ok", func(t *testing.T) {
		ts := newTestServer(t)
		rec := ts.post(t, "/release", idRequest{Token: testToken, ID: "missing"})
		assertStatus(t, rec, http.StatusOK)
	})

	t.Run("401 for an unknown token, keeping the lease", func(t *testing.T) {
		ts := newTestServer(t)
		lease := ts.acquire(t, testPublicKey(1))

		rec := ts.post(t, "/release", idRequest{Token: "not-a-valid-token", ID: lease.ID})
		assertStatus(t, rec, http.StatusUnauthorized)
		if _, kept := ts.leases()[lease.ID]; !kept {
			t.Error("lease released with a rejected token")
		}
	})
}

func TestLeaseOwnership(t *testing.T) {
	const (
		inviteToken      = "vpn_invite-token-for-tests"
		otherInviteToken = "vpn_other-invite-token-for-tests"
	)
	pairs := []struct {
		name         string
		owner, other string
	}{
		{"invite lease, another invite", inviteToken, otherInviteToken},
	}

	newServer := func(t *testing.T) *testServer {
		ts := newTestServer(t)
		ts.addInvite(inviteToken, "65f1a2b3c4d5e6f7a8b9c0d1")
		ts.addInvite(otherInviteToken, "65f1a2b3c4d5e6f7a8b9c0d2")
		return ts
	}

	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			t.Run("/renew by another identity answers like a missing lease", func(t *testing.T) {
				ts := newServer(t)
				l := ts.acquireWith(t, pair.owner, testPublicKey(1))
				missing := ts.post(t, "/renew", idRequest{Token: pair.other, ID: "missing"})
				assertStatus(t, missing, http.StatusGone)
				ts.clock.advance(time.Minute)

				rec := ts.post(t, "/renew", idRequest{Token: pair.other, ID: l.ID})
				assertStatus(t, rec, missing.Code)
				if rec.Body.String() != missing.Body.String() {
					t.Errorf("body = %q, want the missing-lease body %q", rec.Body.String(), missing.Body.String())
				}
				if got, want := ts.leases()[l.ID].ExpiresAt, ts.clock.now().Add(defaultTTL-time.Minute); !got.Equal(want) {
					t.Errorf("expires at %v, want %v (not renewed)", got, want)
				}

				assertStatus(t, ts.post(t, "/renew", idRequest{Token: pair.owner, ID: l.ID}), http.StatusOK)
			})

			t.Run("/release by another identity answers like a missing lease", func(t *testing.T) {
				ts := newServer(t)
				l := ts.acquireWith(t, pair.owner, testPublicKey(1))
				missing := ts.post(t, "/release", idRequest{Token: pair.other, ID: "missing"})
				assertStatus(t, missing, http.StatusOK)

				rec := ts.post(t, "/release", idRequest{Token: pair.other, ID: l.ID})
				assertStatus(t, rec, missing.Code)
				if rec.Body.String() != missing.Body.String() {
					t.Errorf("body = %q, want the missing-lease body %q", rec.Body.String(), missing.Body.String())
				}
				if _, kept := ts.leases()[l.ID]; !kept {
					t.Error("lease released by another identity")
				}
				if _, kept := ts.wg.Peers[testPublicKey(1)]; !kept {
					t.Error("peer removed by another identity")
				}

				assertStatus(t, ts.post(t, "/release", idRequest{Token: pair.owner, ID: l.ID}), http.StatusOK)
				if _, kept := ts.leases()[l.ID]; kept {
					t.Error("lease still stored after its owner released it")
				}
			})
		})
	}
}

// fakeInvites answers invite lookups from records, or fails with err while it is set; any
// other store call panics on the nil embedded store.
type fakeInvites struct {
	store.Store
	records map[string]store.InviteRecord // token hash -> invite
	err     error
	calls   int
}

func (f *fakeInvites) FindInvite(_ context.Context, hash string) (*store.InviteRecord, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	record, ok := f.records[hash]
	if !ok {
		return nil, nil
	}
	return &record, nil
}

// inviteCached reports whether the authenticator answers token without a store lookup.
func (ts *testServer) inviteCached(t *testing.T, token string) bool {
	t.Helper()
	lookups := ts.invites.calls
	if _, _, err := ts.auth.Identify(context.Background(), token); err != nil {
		t.Fatalf("identifying the invite: %v", err)
	}
	return ts.invites.calls == lookups
}

func TestInviteLookupFailure(t *testing.T) {
	const (
		inviteToken      = "vpn_invite-token-for-tests"
		inviteID         = "65f1a2b3c4d5e6f7a8b9c0d1"
		lookupFailedBody = "não consegui verificar o convite agora; tente de novo em instantes"
	)
	errLookup := errors.New("server selection error: internal detail")

	newServer := func(t *testing.T) (*testServer, *fakeInvites) {
		t.Helper()
		ts := newTestServer(t)
		ts.addInvite(inviteToken, inviteID)
		return ts, ts.invites
	}

	assertLookupFailed := func(t *testing.T, rec *httptest.ResponseRecorder, logged *bytes.Buffer) {
		t.Helper()
		assertStatus(t, rec, http.StatusServiceUnavailable)
		if got := strings.TrimSpace(rec.Body.String()); got != lookupFailedBody {
			t.Errorf("body = %q, want the fixed lookup failure message", got)
		}
		if !strings.Contains(logged.String(), "internal detail") {
			t.Errorf("log = %q, want the lookup error", logged.String())
		}
	}

	t.Run("/lease answers 503, and the failure isn't cached", func(t *testing.T) {
		ts, invites := newServer(t)
		logged := captureLog(t)
		invites.err = errLookup

		rec := ts.post(t, "/lease", acquireRequest{Token: inviteToken, PubKey: testPublicKey(1)})
		assertLookupFailed(t, rec, logged)

		invites.err = nil
		ts.acquireWith(t, inviteToken, testPublicKey(1))
	})

	t.Run("/renew answers 503 without renewing, and the failure isn't cached", func(t *testing.T) {
		ts, invites := newServer(t)
		logged := captureLog(t)
		l := ts.acquireWith(t, inviteToken, testPublicKey(1))
		ts.auth.Invalidate(inviteID)
		ts.clock.advance(time.Minute)
		invites.err = errLookup

		rec := ts.post(t, "/renew", idRequest{Token: inviteToken, ID: l.ID})
		assertLookupFailed(t, rec, logged)
		if got, want := ts.leases()[l.ID].ExpiresAt, ts.clock.now().Add(defaultTTL-time.Minute); !got.Equal(want) {
			t.Errorf("expires at %v, want %v (not renewed)", got, want)
		}

		invites.err = nil
		assertStatus(t, ts.post(t, "/renew", idRequest{Token: inviteToken, ID: l.ID}), http.StatusOK)
	})

	t.Run("/release answers 503 keeping the lease, and the failure isn't cached", func(t *testing.T) {
		ts, invites := newServer(t)
		logged := captureLog(t)
		l := ts.acquireWith(t, inviteToken, testPublicKey(1))
		ts.auth.Invalidate(inviteID)
		invites.err = errLookup

		rec := ts.post(t, "/release", idRequest{Token: inviteToken, ID: l.ID})
		assertLookupFailed(t, rec, logged)
		if _, kept := ts.leases()[l.ID]; !kept {
			t.Error("lease released without a lookup")
		}

		invites.err = nil
		assertStatus(t, ts.post(t, "/release", idRequest{Token: inviteToken, ID: l.ID}), http.StatusOK)
		if _, kept := ts.leases()[l.ID]; kept {
			t.Error("lease still stored after the lookup recovered")
		}
	})

	t.Run("a cached invite keeps working while lookups fail", func(t *testing.T) {
		ts, invites := newServer(t)
		l := ts.acquireWith(t, inviteToken, testPublicKey(1))
		invites.err = errLookup

		assertStatus(t, ts.post(t, "/renew", idRequest{Token: inviteToken, ID: l.ID}), http.StatusOK)
	})

	t.Run("an unknown invite gets 401, cached", func(t *testing.T) {
		ts, invites := newServer(t)
		for range 2 {
			rec := ts.post(t, "/lease", acquireRequest{Token: "vpn_unknown-token-for-tests", PubKey: testPublicKey(1)})
			assertStatus(t, rec, http.StatusUnauthorized)
		}
		if invites.calls != 1 {
			t.Errorf("lookups = %d, want 1 (the refusal cached)", invites.calls)
		}
	})
}

// wgBudget is how long work that makes no wg call of its own may take while another
// operation's wg call is held.
const wgBudget = 2 * time.Second

// runAsync runs fn on its own goroutine; the returned channel closes when fn returns.
func runAsync(fn func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	return done
}

func finishesWithinBudget(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	case <-time.After(wgBudget):
		return false
	}
}

func expectStatus(rec *httptest.ResponseRecorder, want int) error {
	if rec.Code != want {
		return fmt.Errorf("status = %d, want %d (body %q)", rec.Code, want, rec.Body.String())
	}
	return nil
}

func TestWireguardCallsDontBlockOtherWork(t *testing.T) {
	const panelSecret = "panel-secret-for-tests"
	const inviteToken = "vpn_invite-token-for-tests"
	const inviteID = "65f1a2b3c4d5e6f7a8b9c0d1"
	heldKey := testPublicKey(1)

	// Each blocker prepares, in the test goroutine, an operation whose wg call is on heldKey.
	blockers := []struct {
		name  string
		setup func(t *testing.T, ts *testServer) func()
	}{
		{"adding a peer in /lease", func(t *testing.T, ts *testServer) func() {
			req := postRequest(t, "/lease", acquireRequest{Token: testToken, PubKey: heldKey})
			return func() { ts.serve(req) }
		}},
		{"removing a peer in /release", func(t *testing.T, ts *testServer) func() {
			l := ts.acquire(t, heldKey)
			req := postRequest(t, "/release", idRequest{Token: testToken, ID: l.ID})
			return func() { ts.serve(req) }
		}},
		{"removing a peer on admin disconnect", func(t *testing.T, ts *testServer) func() {
			l := ts.acquire(t, heldKey)
			req := adminRequest(http.MethodPost, disconnectPath(l.ID), panelSecret)
			return func() { ts.serve(req) }
		}},
		{"removing the peers of a revoked invite", func(t *testing.T, ts *testServer) func() {
			ts.addInvite(inviteToken, inviteID)
			ts.acquireWith(t, inviteToken, heldKey)
			return func() { ts.registry.DisconnectInvite(inviteID) }
		}},
		{"removing an expired peer", func(t *testing.T, ts *testServer) func() {
			// Granted one TTL and a second ago, so it expired a second ago.
			ts.clock.advance(-defaultTTL - time.Second)
			ts.acquire(t, heldKey)
			ts.clock.advance(defaultTTL + time.Second)
			return ts.registry.ExpireStale
		}},
	}

	// Each observer prepares work that needs s.mu but no wg call on heldKey; other is a
	// lease granted before the blocker started.
	observers := []struct {
		name  string
		setup func(t *testing.T, ts *testServer, other acquireResponse) func() error
	}{
		{"/renew", func(t *testing.T, ts *testServer, other acquireResponse) func() error {
			req := postRequest(t, "/renew", idRequest{Token: testToken, ID: other.ID})
			return func() error { return expectStatus(ts.serve(req), http.StatusOK) }
		}},
		{"/lease", func(t *testing.T, ts *testServer, _ acquireResponse) func() error {
			req := postRequest(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(3)})
			return func() error { return expectStatus(ts.serve(req), http.StatusOK) }
		}},
		{"the reaper", func(_ *testing.T, ts *testServer, _ acquireResponse) func() error {
			return func() error {
				ts.registry.ExpireStale()
				return nil
			}
		}},
	}

	for _, b := range blockers {
		for _, o := range observers {
			t.Run(o.name+" proceeds while "+b.name+" is held", func(t *testing.T) {
				ts := newTestServer(t)
				ts.config.PanelSecret = panelSecret
				other := ts.acquire(t, testPublicKey(2))
				blocked := b.setup(t, ts)
				observe := o.setup(t, ts, other)

				entered, release := ts.wg.Hold(t, heldKey)
				blockedDone := runAsync(blocked)
				if !finishesWithinBudget(entered) {
					t.Fatal("the blocker never reached wg")
				}

				var err error
				observed := runAsync(func() { err = observe() })
				stalled := !finishesWithinBudget(observed)
				release()
				<-blockedDone
				<-observed

				if stalled {
					t.Fatalf("%s stalled while %s was held", o.name, b.name)
				}
				if err != nil {
					t.Error(err)
				}
			})
		}
	}
}

func TestReleasedAddressWaitsForPeerRemoval(t *testing.T) {
	ts := newTestServer(t)
	first := ts.acquire(t, testPublicKey(1))

	entered, release := ts.wg.Hold(t, testPublicKey(1))
	releaseReq := postRequest(t, "/release", idRequest{Token: testToken, ID: first.ID})
	released := runAsync(func() { ts.serve(releaseReq) })
	if !finishesWithinBudget(entered) {
		t.Fatal("/release never reached wg")
	}

	var during *httptest.ResponseRecorder
	leaseReq := postRequest(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(2)})
	leased := runAsync(func() { during = ts.serve(leaseReq) })
	stalled := !finishesWithinBudget(leased)
	release()
	<-released
	<-leased
	if stalled {
		t.Fatal("/lease stalled while /release's peer removal was held")
	}

	assertStatus(t, during, http.StatusOK)
	var resp acquireResponse
	if err := json.Unmarshal(during.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Address != "10.9.0.11/32" {
		t.Errorf("address during the removal = %q, want 10.9.0.11/32 while the old peer still holds 10.9.0.10", resp.Address)
	}

	if after := ts.acquire(t, testPublicKey(3)); after.Address != "10.9.0.10/32" {
		t.Errorf("address after the removal = %q, want the freed 10.9.0.10/32", after.Address)
	}
}

// Meant for -race: clients lease, renew and release while the reaper runs.
func TestConcurrentLeasesLeaveNothingBehind(t *testing.T) {
	ts := newTestServer(t)
	const clients, rounds = 20, 10

	stop := make(chan struct{})
	background := runAsync(func() {
		for {
			select {
			case <-stop:
				return
			default:
				ts.registry.ExpireStale()
			}
		}
	})

	var wg sync.WaitGroup
	for n := range clients {
		leaseBody, err := json.Marshal(acquireRequest{Token: testToken, PubKey: testPublicKey(byte(n + 1))})
		if err != nil {
			t.Fatalf("encoding request body: %v", err)
		}
		wg.Go(func() {
			for range rounds {
				rec := ts.serve(httptest.NewRequest(http.MethodPost, "/lease", bytes.NewReader(leaseBody)))
				if err := expectStatus(rec, http.StatusOK); err != nil {
					t.Errorf("/lease: %v", err)
					return
				}
				var resp acquireResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Errorf("decoding /lease response: %v", err)
					return
				}
				idBody := fmt.Sprintf(`{"token":%q,"id":%q}`, testToken, resp.ID)
				for _, path := range []string{"/renew", "/release"} {
					rec := ts.serve(httptest.NewRequest(http.MethodPost, path, strings.NewReader(idBody)))
					if err := expectStatus(rec, http.StatusOK); err != nil {
						t.Errorf("%s: %v", path, err)
						return
					}
				}
			}
		})
	}
	wg.Wait()
	close(stop)
	<-background

	if len(ts.leases()) != 0 || len(ts.wg.Peers) != 0 {
		t.Errorf("left behind: leases %v, peers %v", ts.leases(), ts.wg.Peers)
	}
	ts.assertNoReservedAddresses(t)
}

func TestInviteLeaseCap(t *testing.T) {
	const cappedToken, cappedInviteID = "vpn_capped-invite-for-tests", "65f1a2b3c4d5e6f7a8b9c0d3"
	ts := newTestServer(t)
	ts.addInvite(cappedToken, cappedInviteID)
	record := ts.invites.records[auth.HashToken(cappedToken)]
	maxLeases := 2
	record.MaxLeases = &maxLeases
	ts.invites.records[auth.HashToken(cappedToken)] = record

	first := ts.acquireWith(t, cappedToken, testPublicKey(1))
	ts.acquireWith(t, cappedToken, testPublicKey(2))

	rec := ts.post(t, "/lease", acquireRequest{Token: cappedToken, PubKey: testPublicKey(3)})
	assertStatus(t, rec, http.StatusTooManyRequests)
	if !strings.Contains(rec.Body.String(), "limite de conexões simultâneas") {
		t.Errorf("body = %q, want the invite cap message", rec.Body.String())
	}

	// Other invites don't count toward the cap.
	ts.acquire(t, testPublicKey(4))

	assertStatus(t, ts.post(t, "/release", idRequest{Token: cappedToken, ID: first.ID}), http.StatusOK)
	ts.acquireWith(t, cappedToken, testPublicKey(3))
	ts.assertNoReservedAddresses(t)
}

func TestRefusedTokensBlockTheAddress(t *testing.T) {
	ts := newTestServer(t)

	for i := range maxFailures {
		rec := ts.post(t, "/renew", idRequest{Token: fmt.Sprintf("vpn_made-up-%d", i), ID: "missing"})
		assertStatus(t, rec, http.StatusUnauthorized)
	}
	lookups := ts.invites.calls

	rec := ts.post(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(1)})
	assertStatus(t, rec, http.StatusTooManyRequests)
	if ts.invites.calls != lookups {
		t.Error("a blocked address still reached the database")
	}

	other := postRequest(t, "/lease", acquireRequest{Token: testToken, PubKey: testPublicKey(1)})
	other.RemoteAddr = "198.51.100.7:4000"
	assertStatus(t, ts.serve(other), http.StatusOK)

	ts.clock.advance(failureWindow)
	ts.acquire(t, testPublicKey(2))
}

func TestHealth(t *testing.T) {
	t.Run("answers 200 while the interface is readable", func(t *testing.T) {
		ts := newTestServer(t)
		rec := ts.serve(httptest.NewRequest(http.MethodGet, "/health", nil))
		assertStatus(t, rec, http.StatusOK)
	})

	t.Run("answers 503 when wg can't read the interface", func(t *testing.T) {
		ts := newTestServer(t)
		captureLog(t)
		ts.wg.DumpErr = errors.New("wg show failed")
		rec := ts.serve(httptest.NewRequest(http.MethodGet, "/health", nil))
		assertStatus(t, rec, http.StatusServiceUnavailable)
	})
}
