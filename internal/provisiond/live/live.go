// Package live streams the collector's snapshots to the panel's browser over SSE, straight
// from memory and without going through Vercel or the database. The route is public, so it
// takes a signed, short-lived, read-only ticket.
package live

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/metrics"
)

// maxStreamDuration ends each connection so an expired ticket can't hold a stream forever;
// the panel reconnects with a new ticket.
const maxStreamDuration = 10 * time.Minute

type subscriber struct {
	ch chan metrics.Snapshot
}

type Hub struct {
	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
	closed      bool
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[*subscriber]struct{})}
}

// subscribe returns a subscriber whose channel closes when it unsubscribes or the hub closes.
func (h *Hub) subscribe() (*subscriber, func()) {
	s := &subscriber{ch: make(chan metrics.Snapshot, 1)}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(s.ch)
		return s, func() {}
	}
	h.subscribers[s] = struct{}{}

	return s, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subscribers[s]; ok {
			delete(h.subscribers, s)
			close(s.ch)
		}
	}
}

// Close ends every stream, and any stream that starts afterwards, so a server shutdown
// doesn't wait on them.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subscribers {
		delete(h.subscribers, s)
		close(s.ch)
	}
}

// Publish hands the snapshot to every subscriber without blocking: a slow browser must not
// hold the collector's tick, and a subscriber that hasn't read the previous snapshot skips
// this one.
func (h *Hub) Publish(snapshot metrics.Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for s := range h.subscribers {
		select {
		case s.ch <- snapshot:
		default:
		}
	}
}

// Handler serves the collector's snapshots to the panel, authorized by a ticket signed with secret.
type Handler struct {
	secret      string
	panelOrigin string
	now         func() time.Time
	collector   *metrics.Collector
	hub         *Hub
}

func NewHandler(secret, panelOrigin string, now func() time.Time, collector *metrics.Collector, hub *Hub) *Handler {
	return &Handler{secret: secret, panelOrigin: panelOrigin, now: now, collector: collector, hub: hub}
}

// AllowPanelOrigin lets the panel's browser call next from the panel's origin.
func (h *Handler) AllowPanelOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.panelOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", h.panelOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		next.ServeHTTP(w, r)
	})
}

// Preflight answers the check the browser sends before Stream, because the stream request
// carries an Authorization header.
func Preflight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	t, err := auth.ValidateTicketAt(h.secret, auth.BearerToken(r), h.now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if t.Role != "admin" {
		http.Error(w, "somente administradores", http.StatusForbidden)
		return
	}

	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}

	// The server's 15s WriteTimeout would cut the stream; lift it for this connection only.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("live: removing the write deadline: %v", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	// Turns off nginx buffering, should one ever sit in front; Caddy flushes per its Caddyfile.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	s, unsubscribe := h.hub.subscribe()
	defer unsubscribe()

	if !send(w, fl, h.collector.Current()) {
		return
	}

	limit := time.NewTimer(maxStreamDuration)
	defer limit.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case <-limit.C:
			fmt.Fprint(w, "event: end\ndata: {\"reason\":\"renew the ticket\"}\n\n")
			fl.Flush()
			return

		case snapshot, open := <-s.ch:
			if !open {
				return
			}
			if !send(w, fl, snapshot) {
				return
			}
		}
	}
}

func send(w http.ResponseWriter, fl http.Flusher, snapshot metrics.Snapshot) bool {
	body, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("live: encoding the snapshot: %v", err)
		return false
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
		return false
	}
	fl.Flush()
	return true
}
