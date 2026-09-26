// Package profiletest provides a fake provisiond for tests of packages that
// acquire leases.
package profiletest

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Provisiond answers /lease, /renew and /release over pinned TLS and records
// every request body by path. Set the exported knobs before the first request.
type Provisiond struct {
	URL string

	srv *httptest.Server

	mu       sync.Mutex
	requests map[string][]map[string]string

	LeaseStatus int
	LeaseBody   []byte // served verbatim by /lease with LeaseStatus when set
	RenewStatus int
	TTLSeconds  int

	Renewed  chan struct{}
	Released chan struct{}
}

func NewProvisiond(t testing.TB) *Provisiond {
	t.Helper()
	p := &Provisiond{
		requests:    map[string][]map[string]string{},
		LeaseStatus: http.StatusOK,
		RenewStatus: http.StatusOK,
		TTLSeconds:  180,
		Renewed:     make(chan struct{}, 16),
		Released:    make(chan struct{}, 16),
	}
	p.srv = httptest.NewUnstartedServer(http.HandlerFunc(p.handle))
	// Refused handshakes are the point of some tests; keep them out of the output.
	p.srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	p.srv.StartTLS()
	p.URL = p.srv.URL
	t.Cleanup(p.srv.Close)
	return p
}

// Fingerprint is the pin a client needs to reach this server.
func (p *Provisiond) Fingerprint() string {
	sum := sha256.Sum256(p.srv.Certificate().Raw)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Calls returns the request bodies received on path, in order.
func (p *Provisiond) Calls(path string) []map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]map[string]string(nil), p.requests[path]...)
}

// Close makes the server unreachable before the test ends.
func (p *Provisiond) Close() { p.srv.Close() }

// SetRenewStatus changes RenewStatus while requests may be in flight.
func (p *Provisiond) SetRenewStatus(code int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.RenewStatus = code
}

func (p *Provisiond) handle(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	p.requests[r.URL.Path] = append(p.requests[r.URL.Path], body)
	renewStatus := p.RenewStatus
	p.mu.Unlock()

	switch r.URL.Path {
	case "/lease":
		if p.LeaseBody != nil {
			w.WriteHeader(p.LeaseStatus)
			_, _ = w.Write(p.LeaseBody)
			return
		}
		if p.LeaseStatus != http.StatusOK {
			http.Error(w, "convite inválido", p.LeaseStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "lease-1",
			"address":       "10.8.0.7/32",
			"dns":           "10.8.0.1",
			"server_pubkey": "c2VydmVyLXB1YmxpYy1rZXktcGxhY2Vob2xkZXI=",
			"endpoint":      "203.0.113.10:51820",
			"ttl_seconds":   p.TTLSeconds,
		})
	case "/renew":
		w.WriteHeader(renewStatus)
		p.Renewed <- struct{}{}
	case "/release":
		p.Released <- struct{}{}
	default:
		http.NotFound(w, r)
	}
}
