package lease

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
)

// snapshotVersion is bumped when the file format changes; Restore refuses any other version.
const snapshotVersion = 1

// snapshot carries the leases across a restart. Without it the next renewal of every connected
// client gets a 410, and each one reconnects.
type snapshot struct {
	Version int              `json:"version"`
	Leases  map[string]Lease `json:"leases"`
}

// Save writes the leases to path for the next process to Restore. The file is replaced
// atomically, so a crash mid-write never leaves a truncated one.
func (r *Registry) Save(path string) error {
	data, err := json.Marshal(snapshot{Version: snapshotVersion, Leases: r.Leases()})
	if err != nil {
		return err
	}

	// CreateTemp makes the file 0600: it holds the peers' keys and the devices' names.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // fails harmlessly once renamed

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Restore adopts the leases Save left in path, then removes the file so that no later start
// adopts them again. A missing file adopts nothing. It returns the adopted ids.
//
// A lease is adopted only while unexpired and while its peer is still on the interface,
// holding just the lease's address. Restore never adds a peer: a lease released or
// disconnected after the save lost its peer, so it can't come back.
func (r *Registry) Restore(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil {
		return nil, err
	}

	var saved snapshot
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if saved.Version != snapshotVersion {
		return nil, fmt.Errorf("%s has version %d, want %d", path, saved.Version, snapshotVersion)
	}

	peers, err := r.wg.Dump()
	if err != nil {
		return nil, fmt.Errorf("reading peers on the interface: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	var adopted []string
	for id, l := range saved.Leases {
		if id == "" || !r.adoptable(l, peers, now) {
			log.Printf("saved lease not restored: %s at %s", wireguard.ShortKey(l.PubKey), l.IP)
			continue
		}
		r.leases[id] = &l
		adopted = append(adopted, id)
	}
	return adopted, nil
}

// adoptable reports whether l can be restored. The caller holds r.mu.
func (r *Registry) adoptable(l Lease, peers map[string]wireguard.PeerStat, now time.Time) bool {
	// An expiry beyond one TTL is no renewal's doing.
	if !now.Before(l.ExpiresAt) || l.ExpiresAt.After(now.Add(TTL)) {
		return false
	}

	addr, err := netip.ParseAddr(l.IP)
	if err != nil {
		return false
	}
	address := netip.PrefixFrom(addr, 32)
	if !r.freeLeaseAddresses([]netip.Prefix{address}) || r.leaseAt(l.IP) {
		return false
	}

	p, ok := peers[l.PubKey]
	return ok && len(p.AllowedIPs) == 1 && p.AllowedIPs[0] == address
}

// leaseAt reports whether a stored lease holds ip. The caller holds r.mu.
func (r *Registry) leaseAt(ip string) bool {
	for _, l := range r.leases {
		if l.IP == ip {
			return true
		}
	}
	return false
}
