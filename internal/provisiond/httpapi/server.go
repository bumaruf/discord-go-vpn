package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/lease"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/live"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/metrics"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
)

const (
	leaseFailedMessage   = "não consegui criar o acesso"
	noFreeAddressMessage = "o servidor está sem vagas no momento; tente de novo em alguns minutos"
	// inviteLookupFailedMessage answers a 503 instead of a refusal, so the client retries.
	inviteLookupFailedMessage = "não consegui verificar o convite agora; tente de novo em instantes"
)

type server struct {
	registry *lease.Registry
	wg       wireguard.Interface

	endpoint  string
	dns       string
	publicKey string

	config config.Config
	now    func() time.Time
	random io.Reader

	store store.Store // nil without a database

	collector *metrics.Collector
	hub       *live.Hub
	auth      *auth.Authenticator
	failures  failureLimiter
}

// Options holds what the routes are served from.
type Options struct {
	Config    config.Config
	Registry  *lease.Registry
	WireGuard wireguard.Interface
	Store     store.Store // nil without a database
	Collector *metrics.Collector
	Hub       *live.Hub
	Auth      *auth.Authenticator
	Endpoint  string
	DNS       string
	PublicKey string
}

// New returns the handler of every provisiond route.
func New(o Options) http.Handler {
	s := &server{
		registry:  o.Registry,
		wg:        o.WireGuard,
		endpoint:  o.Endpoint,
		dns:       o.DNS,
		publicKey: o.PublicKey,
		config:    o.Config,
		now:       time.Now,
		random:    rand.Reader,
		store:     o.Store,
		collector: o.Collector,
		hub:       o.Hub,
		auth:      o.Auth,
	}
	return s.routes()
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /lease", s.handleAcquire)
	mux.HandleFunc("POST /renew", s.handleRenew)
	mux.HandleFunc("POST /release", s.handleRelease)
	mux.HandleFunc("GET /health", s.handleHealth)
	stream := live.NewHandler(s.config.LiveSecret, s.config.PanelOrigin, s.now, s.collector, s.hub)
	requireLiveSecret := requireSecret(s.config.LiveSecret, "o stream não está habilitado neste servidor")
	handleGroup(mux, func(next http.Handler) http.Handler { return requireLiveSecret(stream.AllowPanelOrigin(next)) }, map[string]http.Handler{
		"OPTIONS /admin/live": http.HandlerFunc(live.Preflight),
		"GET /admin/live":     http.HandlerFunc(stream.Stream),
	})

	panelSecret := requireBearer(s.config.PanelSecret, "secret inválida")
	requirePanelSecret := requireSecret(s.config.PanelSecret, "as ações do painel não estão habilitadas neste servidor")
	handleGroup(mux, requirePanelSecret, map[string]http.Handler{
		"POST /admin/leases/{id}/disconnect":  panelSecret(http.HandlerFunc(s.handleDisconnect)),
		"POST /admin/invites/{id}/revoke":     panelSecret(http.HandlerFunc(s.handleRevoke)),
		"POST /admin/invites/{id}/reactivate": panelSecret(http.HandlerFunc(s.handleReactivate)),
	})
	return mux
}

type acquireRequest struct {
	Token  string `json:"token"`
	PubKey string `json:"pubkey"`
	Label  string `json:"label"`
	OSUser string `json:"os_user"`
}

type acquireResponse struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	DNS       string `json:"dns"`
	ServerPub string `json:"server_pubkey"`
	Endpoint  string `json:"endpoint"`
	leaseTTL
}

type leaseTTL struct {
	Seconds int `json:"ttl_seconds"`
}

func currentLeaseTTL() leaseTTL {
	return leaseTTL{Seconds: int(lease.TTL.Seconds())}
}

func (s *server) handleAcquire(w http.ResponseWriter, r *http.Request) {
	var p acquireRequest
	if !decodeRequest(w, r, &p) {
		return
	}
	invite, ok := s.identifyInvite(w, r, p.Token, "convite inválido ou revogado")
	if !ok {
		return
	}
	if !validKey(p.PubKey) {
		http.Error(w, "chave pública inválida", http.StatusBadRequest)
		return
	}

	id, err := newID(s.random)
	if err != nil {
		log.Printf("generating lease id: %v", err)
		http.Error(w, leaseFailedMessage, http.StatusInternalServerError)
		return
	}

	device := store.Device{InviteID: invite.ID, Label: cleanLabel(p.Label), OSUser: cleanLabel(p.OSUser)}

	// The request's address, since the peer's endpoint only shows up after its first handshake.
	device.Origin = hostOf(r.RemoteAddr)

	ip, err := s.registry.Reserve(p.PubKey, invite.ID, invite.MaxLeases)
	if err != nil {
		status, message := reserveFailure(err)
		http.Error(w, message, status)
		return
	}

	// Registering waits on Mongo, so it runs outside the registry's lock, and only once
	// nothing can refuse the lease: a refused request must not count as a device access.
	deviceID := s.registerDevice(r.Context(), device)

	s.registry.Grant(id, lease.Lease{
		PubKey:   p.PubKey,
		IP:       ip,
		Label:    device.Label,
		OSUser:   device.OSUser,
		InviteID: invite.ID,
		DeviceID: deviceID,
	})
	log.Printf("lease granted: %s at %s", wireguard.ShortKey(p.PubKey), ip)

	respond(w, acquireResponse{
		ID:        id,
		Address:   ip + "/32",
		DNS:       s.dns,
		ServerPub: s.publicKey,
		Endpoint:  s.endpoint,
		leaseTTL:  currentLeaseTTL(),
	})
}

type idRequest struct {
	Token string `json:"token"`
	ID    string `json:"id"`
}

func (s *server) handleRenew(w http.ResponseWriter, r *http.Request) {
	var p idRequest
	if !decodeRequest(w, r, &p) {
		return
	}
	// Renewing checks the token again, so a revoked invite's lease expires within one TTL
	// even if nobody disconnects it.
	invite, ok := s.identifyInvite(w, r, p.Token, "token inválido")
	if !ok {
		return
	}

	if !s.registry.Renew(p.ID, invite.ID) {
		http.Error(w, "acesso expirado", http.StatusGone)
		return
	}
	respond(w, currentLeaseTTL())
}

func (s *server) handleRelease(w http.ResponseWriter, r *http.Request) {
	var p idRequest
	if !decodeRequest(w, r, &p) {
		return
	}
	invite, ok := s.identifyInvite(w, r, p.Token, "token inválido")
	if !ok {
		return
	}

	if l, ok := s.registry.Release(p.ID, invite.ID); ok {
		log.Printf("lease released: %s", wireguard.ShortKey(l.PubKey))
	}
	respond(w, map[string]bool{"ok": true})
}

// handleHealth leaves the database out: a new process can't fix an Atlas outage, so the
// deploy would roll back a good image for nothing.
func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	if _, err := s.wg.Dump(); err != nil {
		log.Printf("health check: reading the WireGuard interface: %v", err)
		http.Error(w, "wireguard interface unavailable", http.StatusServiceUnavailable)
		return
	}
	respond(w, map[string]bool{"ok": true})
}

// identifyInvite resolves token to its invite. When it can't, it answers 401 with refusal,
// 503 when the database couldn't answer, or 429 to an address refused too often.
func (s *server) identifyInvite(w http.ResponseWriter, r *http.Request, token, refusal string) (invite auth.Invite, ok bool) {
	addr := hostOf(r.RemoteAddr)
	if s.failures.blocked(addr, s.now()) {
		http.Error(w, "tentativas demais com um token inválido; tente de novo em um minuto", http.StatusTooManyRequests)
		return auth.Invite{}, false
	}

	invite, ok, err := s.auth.Identify(r.Context(), token)
	if err != nil {
		log.Printf("invite lookup failed: %v", err)
		http.Error(w, inviteLookupFailedMessage, http.StatusServiceUnavailable)
		return auth.Invite{}, false
	}
	if !ok {
		s.failures.fail(addr, s.now())
		http.Error(w, refusal, http.StatusUnauthorized)
	}
	return invite, ok
}

// reserveFailure maps a refused reservation to its status and the message the client reads.
func reserveFailure(err error) (status int, message string) {
	switch {
	case errors.Is(err, lease.ErrLimitReached):
		return http.StatusTooManyRequests, "limite de acessos simultâneos atingido"
	case errors.Is(err, lease.ErrInviteLimitReached):
		return http.StatusTooManyRequests, "este convite já está no limite de conexões simultâneas"
	case errors.Is(err, lease.ErrNoFreeAddress):
		return http.StatusServiceUnavailable, noFreeAddressMessage
	default:
		return http.StatusInternalServerError, leaseFailedMessage
	}
}

// cleanLabel drops control characters and caps the label at 32 bytes, since it ends up in
// logs and on the panel. An empty label is valid: older clients send none.
func cleanLabel(raw string) string {
	const maxLabel = 32

	var b strings.Builder
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= maxLabel {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

// hostOf strips the port from "ip:port", returning address unchanged when it has none.
func hostOf(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}

func validKey(pub string) bool {
	raw, err := base64.StdEncoding.DecodeString(pub)
	return err == nil && len(raw) == 32
}

func newID(random io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(dst); err != nil {
		http.Error(w, "pedido malformado", http.StatusBadRequest)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
