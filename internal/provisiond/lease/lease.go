package lease

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
)

const (
	TTL       = 3 * time.Minute
	reapEvery = 30 * time.Second

	// Host numbers within the /24 subnet. The range starts at .10 to leave the first addresses
	// for fixed profiles made by hand on the server.
	FirstIP = 10
	LastIP  = 250
)

var (
	ErrLimitReached       = errors.New("active lease limit reached")
	ErrInviteLimitReached = errors.New("the invite's lease limit reached")
	ErrNoFreeAddress      = errors.New("no free address in the range")
)

// Lease is saved across restarts (snapshot.go), so its JSON names are a file format.
type Lease struct {
	PubKey    string    `json:"pubkey"`
	IP        string    `json:"ip"`
	ExpiresAt time.Time `json:"expires_at"`
	Label     string    `json:"label"`
	OSUser    string    `json:"os_user"`
	// Since is when the lease was granted; ExpiresAt moves forward on every renewal.
	Since    time.Time `json:"since"`
	InviteID string    `json:"invite_id"`
	// DeviceID outlives the lease, so usage history follows the machine rather than the peer.
	DeviceID string `json:"device_id"`
}

// Registry holds the leases in memory and the peers they keep on the interface.
type Registry struct {
	mu     sync.Mutex
	leases map[string]*Lease // id -> lease

	// reservedIPs holds the addresses whose peer is being added or removed outside r.mu, or
	// whose granted lease is registering its device; they count as leases for the limits and freeIP.
	// The value is the invite a new lease is reserved for, empty for a peer being removed.
	reservedIPs map[string]string

	subnet    netip.Prefix
	maxActive int
	wg        wireguard.Interface
	now       func() time.Time
}

// NewRegistry hands out addresses from subnet, which must be an IPv4 /24.
func NewRegistry(wg wireguard.Interface, subnet netip.Prefix, now func() time.Time, maxActive int) *Registry {
	return &Registry{
		leases:      make(map[string]*Lease),
		reservedIPs: make(map[string]string),
		subnet:      subnet,
		maxActive:   maxActive,
		wg:          wg,
		now:         now,
	}
}

// Reserve reserves an address for a lease of inviteID and adds its peer, without r.mu held
// during wg. maxLeases caps the invite's leases, reservations included; 0 means no cap. On
// success the address stays in reservedIPs until Grant stores the lease.
func (r *Registry) Reserve(pubKey, inviteID string, maxLeases int) (ip string, err error) {
	ip, err = r.reserveAddress(inviteID, maxLeases)
	if err != nil {
		return "", err
	}

	if err := r.wg.AddPeer(pubKey, ip); err != nil {
		log.Printf("adding peer: %v", err)
		r.releaseAddress(ip)
		return "", fmt.Errorf("adding peer: %w", err)
	}
	return ip, nil
}

// Grant stores the lease reserved for l.IP under id, starting its TTL now.
func (r *Registry) Grant(id string, l Lease) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.reservedIPs, l.IP)

	now := r.now()
	l.ExpiresAt = now.Add(TTL)
	l.Since = now
	r.leases[id] = &l
}

// Renew pushes the expiry of the lease one TTL from now, and reports false when inviteID
// has no such lease.
func (r *Registry) Renew(id, inviteID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	l := r.ownedLease(id, inviteID)
	if l == nil {
		return false
	}
	l.ExpiresAt = r.now().Add(TTL)
	return true
}

// Release removes the lease and its peer, and reports false when inviteID has no such lease.
func (r *Registry) Release(id, inviteID string) (Lease, bool) {
	return first(r.removeWhere(func(leaseID string, l *Lease) bool {
		return leaseID == id && l.InviteID == inviteID
	}))
}

// Disconnect removes the lease and its peer whoever owns it, and reports false when there
// is no such lease.
func (r *Registry) Disconnect(id string) (Lease, bool) {
	return first(r.removeWhere(func(leaseID string, _ *Lease) bool { return leaseID == id }))
}

// DisconnectInvite removes every lease of the invite and returns how many there were.
func (r *Registry) DisconnectInvite(inviteID string) int {
	return len(r.removeWhere(func(_ string, l *Lease) bool { return l.InviteID == inviteID }))
}

// Leases returns a copy of the stored leases, keyed by id.
func (r *Registry) Leases() map[string]Lease {
	r.mu.Lock()
	defer r.mu.Unlock()

	leases := make(map[string]Lease, len(r.leases))
	for id, l := range r.leases {
		leases[id] = *l
	}
	return leases
}

func (r *Registry) MaxActive() int {
	return r.maxActive
}

// ownedLease returns the lease only when it was acquired by the identity inviteID. Another
// identity's lease is nil, like a missing one, so a caller can't learn that the id exists.
// The caller holds r.mu.
func (r *Registry) ownedLease(id, inviteID string) *Lease {
	l, ok := r.leases[id]
	if !ok || l.InviteID != inviteID {
		return nil
	}
	return l
}

// removeWhere takes the matching leases out of r.leases, then removes their peers without
// r.mu held. Each address stays reserved until its peer is gone, so it can't go to a new
// peer while the old one holds it.
func (r *Registry) removeWhere(match func(id string, l *Lease) bool) []Lease {
	r.mu.Lock()
	var removed []Lease
	for id, l := range r.leases {
		if match(id, l) {
			delete(r.leases, id)
			r.reservedIPs[l.IP] = ""
			removed = append(removed, *l)
		}
	}
	r.mu.Unlock()

	for _, l := range removed {
		if err := r.wg.RemovePeer(l.PubKey); err != nil {
			log.Printf("removing peer %s: %v", wireguard.ShortKey(l.PubKey), err)
		}
		r.releaseAddress(l.IP)
	}
	return removed
}

func first(leases []Lease) (Lease, bool) {
	if len(leases) == 0 {
		return Lease{}, false
	}
	return leases[0], true
}

// reserveAddress runs every check that can refuse a lease and reserves a free address.
func (r *Registry) reserveAddress(inviteID string, maxLeases int) (string, error) {
	r.ExpireStale()

	onInterface, err := r.addressesOnInterface()
	if err != nil {
		log.Printf("reading peers on the interface: %v", err)
		return "", fmt.Errorf("reading peers on the interface: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.leases)+len(r.reservedIPs) >= r.maxActive {
		return "", ErrLimitReached
	}
	if maxLeases > 0 && r.inviteLeases(inviteID) >= maxLeases {
		return "", ErrInviteLimitReached
	}

	ip, err := r.freeIP(onInterface)
	if err != nil {
		log.Printf("reserving lease address: %v", err)
		return "", err
	}

	r.reservedIPs[ip] = inviteID
	return ip, nil
}

// inviteLeases counts the invite's leases and the addresses reserved for new ones. The caller
// holds r.mu.
func (r *Registry) inviteLeases(inviteID string) int {
	count := 0
	for _, l := range r.leases {
		if l.InviteID == inviteID {
			count++
		}
	}
	for _, reservedFor := range r.reservedIPs {
		if reservedFor == inviteID {
			count++
		}
	}
	return count
}

func (r *Registry) releaseAddress(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.reservedIPs, ip)
}

// addressesOnInterface returns the addresses that peers on the interface hold. A restart that
// couldn't restore the leases leaves the peers of clients still connected on wg0 with no lease,
// and handing out their address would move it to the new peer and cut them off.
func (r *Registry) addressesOnInterface() (map[string]bool, error) {
	peers, err := r.wg.Dump()
	if err != nil {
		return nil, err
	}

	held := make(map[string]bool)
	for _, p := range peers {
		for _, prefix := range p.AllowedIPs {
			if prefix.IsSingleIP() {
				held[prefix.Addr().String()] = true
			}
		}
	}
	return held, nil
}

func (r *Registry) freeIP(onInterface map[string]bool) (string, error) {
	used := make(map[string]bool, len(r.leases)+len(r.reservedIPs))
	for _, l := range r.leases {
		used[l.IP] = true
	}
	for ip := range r.reservedIPs {
		used[ip] = true
	}

	octets := r.subnet.Addr().As4()
	for i := FirstIP; i <= LastIP; i++ {
		octets[3] = byte(i)
		ip := netip.AddrFrom4(octets).String()
		if !used[ip] && !onInterface[ip] {
			return ip, nil
		}
	}
	return "", ErrNoFreeAddress
}

// ExpireStale removes the leases past their expiry, so a client that vanished leaves no peer
// behind.
func (r *Registry) ExpireStale() {
	now := r.now()
	for _, l := range r.removeWhere(func(_ string, l *Lease) bool { return now.After(l.ExpiresAt) }) {
		log.Printf("lease expired: %s at %s", wireguard.ShortKey(l.PubKey), l.IP)
	}
}

func (r *Registry) Reaper() {
	t := time.NewTicker(reapEvery)
	defer t.Stop()

	for range t.C {
		r.reap()
	}
}

func (r *Registry) reap() {
	r.ExpireStale()
	r.RemoveOrphans()
}

// orphanIdle is how long a peer without a lease must go without a handshake before it is
// removed; a peer in use handshakes every two minutes.
const orphanIdle = 15 * time.Minute

// RemoveOrphans removes the idle peers on the interface that no lease holds, such as those a
// provisiond that restarted left behind. Peers outside the lease range are made by hand and kept.
func (r *Registry) RemoveOrphans() {
	peers, err := r.wg.Dump()
	if err != nil {
		log.Printf("reading peers on the interface: %v", err)
		return
	}

	for _, p := range r.orphans(peers) {
		if err := r.wg.RemovePeer(p.PubKey); err != nil {
			log.Printf("removing orphan peer %s: %v", wireguard.ShortKey(p.PubKey), err)
			continue
		}
		log.Printf("orphan peer removed: %s at %s", wireguard.ShortKey(p.PubKey), addressList(p.AllowedIPs))
	}
}

func (r *Registry) orphans(peers map[string]wireguard.PeerStat) []wireguard.PeerStat {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()

	leased := make(map[string]bool, len(r.leases))
	for _, l := range r.leases {
		leased[l.PubKey] = true
	}

	var orphans []wireguard.PeerStat
	for _, p := range peers {
		idle := p.Handshake.IsZero() || now.Sub(p.Handshake) > orphanIdle
		if !leased[p.PubKey] && idle && r.freeLeaseAddresses(p.AllowedIPs) {
			orphans = append(orphans, p)
		}
	}
	return orphans
}

// freeLeaseAddresses reports whether every prefix is a single address of the lease range that
// isn't reserved. The caller holds r.mu.
func (r *Registry) freeLeaseAddresses(prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		addr := p.Addr()
		_, reserved := r.reservedIPs[addr.String()]
		if !p.IsSingleIP() || !addr.Is4() || !r.subnet.Contains(addr) || reserved {
			return false
		}
		if host := int(addr.As4()[3]); host < FirstIP || host > LastIP {
			return false
		}
	}
	return true
}

func addressList(prefixes []netip.Prefix) string {
	if len(prefixes) == 0 {
		return "(none)"
	}
	addresses := make([]string, len(prefixes))
	for i, p := range prefixes {
		addresses[i] = p.Addr().String()
	}
	return strings.Join(addresses, ", ")
}
