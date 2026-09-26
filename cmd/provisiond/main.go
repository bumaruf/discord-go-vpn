// provisiond hands out short-lived WireGuard peer leases to clients, which renew them while
// they run, so the peer of a client that dies without releasing it expires on its own.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/httpapi"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/lease"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/live"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/metrics"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/tlscert"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
)

// Together they must fit in the Compose stop_grace_period, or Docker kills provisiond first.
const (
	shutdownTimeout = 10 * time.Second // for the requests in flight
	closeTimeout    = 10 * time.Second // for the queued metric writes and the database
)

func main() {
	var (
		listen    = flag.String("listen", ":51821", "address to listen on")
		iface     = flag.String("interface", "wg0", "WireGuard interface to manage")
		endpoint  = flag.String("endpoint", "", "public tunnel address (ip:port) handed to clients")
		dns       = flag.String("dns", "1.1.1.1", "DNS server handed to clients")
		certFile  = flag.String("cert", "/etc/wireguard/provisiond.crt", "TLS certificate")
		keyFile   = flag.String("key", "/etc/wireguard/provisiond.key", "TLS certificate key")
		maxActive = flag.Int("max", 50, "maximum simultaneous leases")
		leases    = flag.String("leases", "/etc/wireguard/leases.json", "file that carries the leases across a restart")
	)
	flag.Parse()

	cfg := config.Load()
	if cfg.MongoURI == "" {
		log.Fatal("set MONGODB_URI: the invites that authorize the clients live in the database")
	}
	if *endpoint == "" {
		log.Fatal("set -endpoint to the public tunnel address, for example 1.2.3.4:51820")
	}

	subnet, err := cfg.WGSubnetPrefix()
	if err != nil {
		log.Fatal(err)
	}

	wg := wireguard.NewCLI(*iface)
	pub, err := wg.PublicKey()
	if err != nil {
		log.Fatalf("reading the public key of %s: %v", *iface, err)
	}

	registry := lease.NewRegistry(wg, subnet, time.Now, *maxActive)

	if err := tlscert.Ensure(*certFile, *keyFile); err != nil {
		log.Fatalf("preparing the certificate: %v", err)
	}
	fingerprint, err := tlscert.Fingerprint(*certFile)
	if err != nil {
		log.Fatalf("reading the certificate: %v", err)
	}

	// Before the reaper and the routes, so no renewal meets a registry still empty.
	restored, err := registry.Restore(*leases)
	if err != nil {
		log.Printf("restoring the leases (connected clients will reconnect): %v", err)
	}
	log.Printf("leases restored: %d", len(restored))

	go registry.Reaper()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Without the database every invite would get a 401, which makes clients discard their
	// saved token; exiting lets Compose retry while clients only see the server down.
	db, err := store.Open(ctx, cfg)
	if err != nil {
		log.Fatalf("opening the database: %v", err)
	}
	writer := store.NewWriter(db)
	hub := live.NewHub()
	collector := metrics.NewCollector(wg, registry, writer, time.Now, hub.Publish)
	collector.Resume(restored)
	authenticator := auth.NewAuthenticator(db, time.Now)

	log.Printf("provisiond up on %s, interface %s", *listen, *iface)
	log.Printf("certificate fingerprint (the panel's PROVISIOND_FINGERPRINT):\n  %s", fingerprint)

	srv := &http.Server{
		Addr: *listen,
		Handler: httpapi.New(httpapi.Options{
			Config:    cfg,
			Registry:  registry,
			WireGuard: wg,
			Store:     db,
			Collector: collector,
			Hub:       hub,
			Auth:      authenticator,
			Endpoint:  *endpoint,
			DNS:       *dns,
			PublicKey: pub,
		}),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		TLSConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
	}
	srv.RegisterOnShutdown(hub.Close)
	svc := &service{
		http:            srv,
		listen:          func() error { return srv.ListenAndServeTLS(*certFile, *keyFile) },
		shutdownTimeout: shutdownTimeout,
		background:      []func(context.Context){collector.Run},
		saveLeases:      func() error { return registry.Save(*leases) },
		writer:          writer,
		db:              db,
	}
	if err := svc.run(ctx); err != nil {
		log.Fatal(err)
	}
	log.Print("provisiond stopped")
}

// service is what provisiond runs once it is wired.
type service struct {
	http   *http.Server
	listen func() error
	// shutdownTimeout bounds the wait for requests in flight; the connections still open
	// after it are closed.
	shutdownTimeout time.Duration
	background      []func(context.Context) // each runs until its context ends
	saveLeases      func() error            // nil in tests that don't need it
	writer          *store.Writer           // nil without a database
	db              *store.Mongo            // nil without a database
}

// run starts the background work and serves until ctx ends or listen fails. Then it shuts
// the server down, waits for the background work, saves the leases, sends the queued writes
// and closes the database.
func (s *service) run(ctx context.Context) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()

	var background sync.WaitGroup
	for _, work := range s.background {
		background.Go(func() { work(ctx) })
	}

	served := make(chan error, 1)
	go func() { served <- s.listen() }()

	var err error
	select {
	case err = <-served:
		stop()
	case <-ctx.Done():
		log.Print("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutting down: %v; closing the connections still open", err)
			_ = s.http.Close()
		}
	}

	background.Wait()
	// No request can change the leases anymore, and saving comes before the database, whose
	// close may take its whole timeout.
	if s.saveLeases != nil {
		if err := s.saveLeases(); err != nil {
			log.Printf("saving the leases (connected clients will reconnect after the restart): %v", err)
		}
	}
	s.close()
	return err
}

func (s *service) close() {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	if s.writer != nil {
		s.writer.Close(ctx)
	}
	s.db.Close(ctx)
}
