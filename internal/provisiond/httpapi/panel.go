package httpapi

import (
	"log"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
)

// The panel's lists, as it showed them when it read the database itself.
const (
	invitesLimit      = 100
	inviteLabelsLimit = 200
	devicesLimit      = 500
	inviteLabelMax    = 60
)

// usageCacheTTL matches how often the collector writes daily usage: a fresher answer can't differ.
const usageCacheTTL = 30 * time.Second

const noDatabaseMessage = "sem banco: o servidor não tem dados para o painel"

var (
	isoDay    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	tokenHash = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type inviteJSON struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Prefix      string `json:"prefix"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	MaxLeases   *int   `json:"max_leases"`
	DeviceCount int    `json:"device_count"`
}

type invitesResponse struct {
	Invites []inviteJSON `json:"invites"`
}

type deviceJSON struct {
	ID          string `json:"id"`
	InviteID    string `json:"invite_id,omitempty"`
	Name        string `json:"name"`
	OSUser      string `json:"os_user,omitempty"`
	LastIP      string `json:"last_ip,omitempty"`
	FirstSeenAt string `json:"first_seen_at"`
	LastSeenAt  string `json:"last_seen_at"`
	AccessCount int    `json:"access_count"`
}

type devicesResponse struct {
	Devices      []deviceJSON      `json:"devices"`
	InviteLabels map[string]string `json:"invite_labels"`
}

type dayUsageJSON struct {
	Day     string `json:"day"`
	RxBytes int64  `json:"rx_bytes"`
	TxBytes int64  `json:"tx_bytes"`
}

type usageResponse struct {
	Days []dayUsageJSON `json:"days"`
}

type createInviteRequest struct {
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	TokenHash string `json:"token_hash"`
	Prefix    string `json:"prefix"`
	CreatedBy string `json:"created_by"`
	MaxLeases *int   `json:"max_leases"`
}

type maxLeasesRequest struct {
	MaxLeases *int `json:"max_leases"`
}

type auditRequest struct {
	ActorID string `json:"actor_id"`
	Action  string `json:"action"`
	Target  string `json:"target"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail"`
}

// usageCache keeps usage answers for usageCacheTTL, keyed by query.
type usageCache struct {
	mu      sync.Mutex
	entries map[string]usageEntry
}

type usageEntry struct {
	days    []dayUsageJSON
	expires time.Time
}

func (c *usageCache) get(key string, now time.Time) ([]dayUsageJSON, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	return e.days, ok && now.Before(e.expires)
}

func (c *usageCache) put(key string, days []dayUsageJSON, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]usageEntry)
	}
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	c.entries[key] = usageEntry{days: days, expires: now.Add(usageCacheTTL)}
}

// panelData answers 503 and returns false without a database.
func (s *server) panelData(w http.ResponseWriter) (store.PanelData, bool) {
	if s.data == nil {
		http.Error(w, noDatabaseMessage, http.StatusServiceUnavailable)
		return nil, false
	}
	return s.data, true
}

// databaseFailed logs err and answers 503: the panel tells the admin the database didn't answer.
func databaseFailed(w http.ResponseWriter, what string, err error) {
	log.Printf("panel: %s: %v", what, err)
	http.Error(w, "o banco não respondeu", http.StatusServiceUnavailable)
}

func (s *server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	data, ok := s.panelData(w)
	if !ok {
		return
	}
	invites, err := data.ListInvites(r.Context(), invitesLimit)
	if err != nil {
		databaseFailed(w, "listing invites", err)
		return
	}

	resp := invitesResponse{Invites: make([]inviteJSON, len(invites))}
	for i, inv := range invites {
		resp.Invites[i] = inviteJSON{
			ID:          inv.ID,
			Label:       inv.Label,
			Kind:        inv.Kind,
			Prefix:      inv.Prefix,
			Status:      inv.Status,
			CreatedAt:   formatTime(inv.CreatedAt),
			MaxLeases:   inv.MaxLeases,
			DeviceCount: inv.DeviceCount,
		}
	}
	respond(w, resp)
}

func (s *server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	data, ok := s.panelData(w)
	if !ok {
		return
	}
	devices, err := data.ListDevices(r.Context(), devicesLimit)
	if err != nil {
		databaseFailed(w, "listing devices", err)
		return
	}
	labels, err := data.InviteLabels(r.Context(), inviteLabelsLimit)
	if err != nil {
		databaseFailed(w, "listing invite labels", err)
		return
	}

	resp := devicesResponse{Devices: make([]deviceJSON, len(devices)), InviteLabels: labels}
	for i, d := range devices {
		resp.Devices[i] = deviceJSON{
			ID:          d.ID,
			InviteID:    d.InviteID,
			Name:        d.Name,
			OSUser:      d.OSUser,
			LastIP:      d.LastIP,
			FirstSeenAt: formatTime(d.CreatedAt),
			LastSeenAt:  formatTime(d.LastSeenAt),
			AccessCount: d.AccessCount,
		}
	}
	respond(w, resp)
}

// handleUsage sums daily usage from ?from=YYYY-MM-DD on: of the whole server, or of one device
// with ?device=<id>.
func (s *server) handleUsage(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	device := r.URL.Query().Get("device")
	if !isoDay.MatchString(from) {
		http.Error(w, "from precisa ser uma data AAAA-MM-DD", http.StatusBadRequest)
		return
	}
	data, ok := s.panelData(w)
	if !ok {
		return
	}

	key := from + "|" + device
	if days, ok := s.usage.get(key, s.now()); ok {
		respond(w, usageResponse{Days: days})
		return
	}

	rows, err := data.UsageByDay(r.Context(), from, device)
	if err != nil {
		databaseFailed(w, "reading usage", err)
		return
	}
	days := make([]dayUsageJSON, len(rows))
	for i, d := range rows {
		days[i] = dayUsageJSON(d)
	}
	s.usage.put(key, days, s.now())
	respond(w, usageResponse{Days: days})
}

func (s *server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	var p createInviteRequest
	if !decodeRequest(w, r, &p) {
		return
	}
	if p.Label == "" || utf8.RuneCountInString(p.Label) > inviteLabelMax ||
		!slices.Contains([]string{store.InviteIndividual, store.InviteGlobal}, p.Kind) ||
		!tokenHash.MatchString(p.TokenHash) || p.Prefix == "" || p.CreatedBy == "" ||
		(p.MaxLeases != nil && *p.MaxLeases < 1) {
		http.Error(w, "convite inválido", http.StatusBadRequest)
		return
	}
	data, ok := s.panelData(w)
	if !ok {
		return
	}

	err := data.InsertInvite(r.Context(), store.NewInvite{
		Label:     p.Label,
		Kind:      p.Kind,
		TokenHash: p.TokenHash,
		Prefix:    p.Prefix,
		CreatedBy: p.CreatedBy,
		CreatedAt: s.now(),
		MaxLeases: p.MaxLeases,
	})
	if err != nil {
		databaseFailed(w, "creating an invite", err)
		return
	}
	s.respondAction(w, actionResponse{OK: true, Message: "Convite criado", Affected: 1})
}

// handleDeleteInvite removes the invite document only; the panel revokes it first, and its
// devices keep their history.
func (s *server) handleDeleteInvite(w http.ResponseWriter, r *http.Request) {
	data, ok := s.panelData(w)
	if !ok {
		return
	}
	inviteID := r.PathValue("id")
	deleted, err := data.DeleteInvite(r.Context(), inviteID)
	if err != nil {
		databaseFailed(w, "deleting an invite", err)
		return
	}
	if !deleted {
		http.Error(w, "esse convite não existe", http.StatusNotFound)
		return
	}
	s.auth.Invalidate(inviteID)
	s.respondAction(w, actionResponse{OK: true, Message: "Convite excluído", Affected: 1})
}

// handleSetInviteMaxLeases stores the cap and drops the invite from the auth cache, so the next
// lease already checks the new cap. Leases already granted stay.
func (s *server) handleSetInviteMaxLeases(w http.ResponseWriter, r *http.Request) {
	var p maxLeasesRequest
	if !decodeRequest(w, r, &p) {
		return
	}
	if p.MaxLeases != nil && *p.MaxLeases < 1 {
		http.Error(w, "o limite precisa ser positivo", http.StatusBadRequest)
		return
	}
	data, ok := s.panelData(w)
	if !ok {
		return
	}
	inviteID := r.PathValue("id")
	updated, err := data.SetInviteMaxLeases(r.Context(), inviteID, p.MaxLeases)
	if err != nil {
		databaseFailed(w, "setting an invite's lease cap", err)
		return
	}
	if !updated {
		http.Error(w, "esse convite não existe", http.StatusNotFound)
		return
	}
	s.auth.Invalidate(inviteID)
	s.respondAction(w, actionResponse{OK: true, Message: "Limite alterado", Affected: 1})
}

func (s *server) handleAudit(w http.ResponseWriter, r *http.Request) {
	var p auditRequest
	if !decodeRequest(w, r, &p) {
		return
	}
	if p.ActorID == "" || !slices.Contains(store.AuditActions, p.Action) {
		http.Error(w, "registro de auditoria inválido", http.StatusBadRequest)
		return
	}
	data, ok := s.panelData(w)
	if !ok {
		return
	}
	err := data.InsertAudit(r.Context(), store.AuditEntry{
		ActorID: p.ActorID,
		Action:  p.Action,
		Target:  p.Target,
		OK:      p.OK,
		Detail:  p.Detail,
		At:      s.now(),
	})
	if err != nil {
		databaseFailed(w, "recording an audit entry", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
