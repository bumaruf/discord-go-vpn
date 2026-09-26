// Package wireguardtest provides a fake WireGuard interface for tests.
package wireguardtest

import (
	"sync"
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
)

// Fake records peers in memory. Set the exported fields before handing it to concurrent code.
type Fake struct {
	mu      sync.Mutex
	Peers   map[string]string // public key -> tunnel IP
	Stats   map[string]wireguard.PeerStat
	AddErr  error
	DumpErr error
	Removed []string
	gates   map[string]*gate // public key -> gate holding its calls
}

type gate struct {
	entered   chan struct{}
	enterOnce sync.Once
	open      chan struct{}
}

func NewFake() *Fake {
	return &Fake{Peers: make(map[string]string), gates: make(map[string]*gate)}
}

// Hold blocks AddPeer and RemovePeer on publicKey until release is called; entered closes
// when the first such call arrives. Cleanup releases a gate the test left held.
func (f *Fake) Hold(t testing.TB, publicKey string) (entered <-chan struct{}, release func()) {
	t.Helper()
	g := &gate{entered: make(chan struct{}), open: make(chan struct{})}
	f.mu.Lock()
	f.gates[publicKey] = g
	f.mu.Unlock()
	release = sync.OnceFunc(func() { close(g.open) })
	t.Cleanup(release)
	return g.entered, release
}

func (f *Fake) waitGate(publicKey string) {
	f.mu.Lock()
	g := f.gates[publicKey]
	f.mu.Unlock()
	if g == nil {
		return
	}
	g.enterOnce.Do(func() { close(g.entered) })
	<-g.open
}

func (f *Fake) AddPeer(publicKey, ip string) error {
	f.waitGate(publicKey)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.AddErr != nil {
		return f.AddErr
	}
	f.Peers[publicKey] = ip
	return nil
}

func (f *Fake) RemovePeer(publicKey string) error {
	f.waitGate(publicKey)
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Peers, publicKey)
	f.Removed = append(f.Removed, publicKey)
	return nil
}

// Dump returns Stats, or DumpErr when it is set.
func (f *Fake) Dump() (map[string]wireguard.PeerStat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DumpErr != nil {
		return nil, f.DumpErr
	}
	return f.Stats, nil
}
