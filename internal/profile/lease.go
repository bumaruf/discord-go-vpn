package profile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/curve25519"

	"github.com/bumaruf/discord-go-vpn/internal/platform/osuser"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

const (
	renewalsPerTTL = 3
	// minRenewInterval keeps a short TTL from renewing in a tight loop.
	minRenewInterval = 20 * time.Second

	// keepaliveSeconds is WireGuard's usual value for keeping NAT mappings open.
	keepaliveSeconds = 25

	httpTimeout  = 20 * time.Second
	maxErrorBody = 512
	maxReplyBody = 8192
)

type Client struct {
	Server      string // https://host:port
	Token       string
	Fingerprint string // base64 SHA-256 of the server certificate
	Label       string // the device's name in the panel; empty means the hostname

	// newTicker drives renewals; nil means time.NewTicker. Tests replace it.
	newTicker func(d time.Duration) (ticks <-chan time.Time, stop func())
}

func (c *Client) ticker(d time.Duration) (<-chan time.Time, func()) {
	if c.newTicker != nil {
		return c.newTicker(d)
	}
	t := time.NewTicker(d)
	return t.C, t.Stop
}

type leaseRequest struct {
	Token  string `json:"token"`
	PubKey string `json:"pubkey"`
	Label  string `json:"label"`
	OSUser string `json:"os_user,omitempty"`
}

type Lease struct {
	id     string
	http   *http.Client
	client *Client
	ttl    time.Duration
	done   chan struct{}
	once   sync.Once
	Config *wg.Config

	renewFailures chan error
}

// RenewFailures delivers failed renewals. It holds only the latest unread one, so
// renewing never waits on the reader.
func (a *Lease) RenewFailures() <-chan error { return a.renewFailures }

// Acquire leases an address for a fresh key pair and keeps the lease renewed
// until Release.
func (c *Client) Acquire() (*Lease, error) {
	priv, pub, err := keyPair()
	if err != nil {
		return nil, err
	}

	cli, err := c.httpClient()
	if err != nil {
		return nil, err
	}

	var reply struct {
		ID        string `json:"id"`
		Address   string `json:"address"`
		DNS       string `json:"dns"`
		ServerPub string `json:"server_pubkey"`
		Endpoint  string `json:"endpoint"`
		TTL       int    `json:"ttl_seconds"`
	}
	// The label and OS user only identify the device in the panel; they authorize nothing.
	label := c.Label
	if label == "" {
		label, _ = os.Hostname()
	}

	if err := post(cli, c.Server+"/lease", leaseRequest{
		Token: c.Token, PubKey: pub, Label: label, OSUser: osuser.Name(),
	}, &reply); err != nil {
		return nil, err
	}

	cfg, err := buildConfig(priv, reply.ServerPub, reply.Address, reply.DNS, reply.Endpoint)
	if err != nil {
		return nil, err
	}

	a := &Lease{
		id:     reply.ID,
		http:   cli,
		client: c,
		ttl:    time.Duration(reply.TTL) * time.Second,
		done:   make(chan struct{}),
		Config: cfg,

		renewFailures: make(chan error, 1),
	}
	go a.keepAlive()
	return a, nil
}

// keepAlive renews well before the TTL runs out, so a network hiccup does not
// lose the lease.
func (a *Lease) keepAlive() {
	interval := a.ttl / renewalsPerTTL
	if interval < minRenewInterval {
		interval = minRenewInterval
	}

	ticks, stop := a.client.ticker(interval)
	defer stop()

	failures := 0
	for {
		select {
		case <-a.done:
			return
		case <-ticks:
			err := post(a.http, a.client.Server+"/renew", map[string]string{
				"token": a.client.Token, "id": a.id,
			}, nil)
			if err == nil {
				failures = 0
				continue
			}
			failures++
			a.reportRenewFailure(renewalError(err, time.Duration(failures)*interval >= a.ttl))
		}
	}
}

// reportRenewFailure replaces an unread failure, since the latest one describes
// the lease now. keepAlive is the only sender, so the send never blocks.
func (a *Lease) reportRenewFailure(err error) {
	select {
	case <-a.renewFailures:
	default:
	}
	a.renewFailures <- err
}

// ErrLeaseLost marks a renewal failure after which the lease, and its peer, no
// longer exist on the server.
var ErrLeaseLost = errors.New("o acesso foi perdido")

// renewalError treats only 410 as proof the lease is gone. Any other failure, such as
// a 503 while provisiond can't reach its database, means lost only once failed
// renewals have spanned the whole TTL.
func renewalError(err error, ttlSpent bool) error {
	var status *StatusError
	if ttlSpent || errors.As(err, &status) && status.StatusCode == http.StatusGone {
		return fmt.Errorf("%w: %w", ErrLeaseLost, err)
	}
	return fmt.Errorf("não consegui renovar o acesso: %w", err)
}

// Release stops renewing and releases the lease on the server. Only the first
// call does anything.
func (a *Lease) Release() {
	a.once.Do(func() {
		close(a.done)
		_ = post(a.http, a.client.Server+"/release", map[string]string{
			"token": a.client.Token, "id": a.id,
		}, nil)
	})
}

func keyPair() (priv, pub string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	// Curve25519 clamping.
	raw[0] &= 248
	raw[31] &= 127
	raw[31] |= 64

	public, err := curve25519.X25519(raw, curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(raw), base64.StdEncoding.EncodeToString(public), nil
}

func buildConfig(priv, serverPub, address, dns, endpoint string) (*wg.Config, error) {
	prefix, err := netip.ParsePrefix(address)
	if err != nil {
		return nil, fmt.Errorf("endereço inválido do servidor: %w", err)
	}
	resolver, err := netip.ParseAddr(dns)
	if err != nil {
		resolver = wg.DefaultDNS
	}

	return &wg.Config{
		PrivateKey: priv,
		Addresses:  []netip.Addr{prefix.Addr()},
		DNS:        []netip.Addr{resolver},
		Peer: wg.Peer{
			PublicKey:  serverPub,
			Endpoint:   endpoint,
			AllowedIPs: wg.DefaultAllowedIPs(),
			Keepalive:  keepaliveSeconds,
		},
	}, nil
}

// httpClient trusts only the certificate with the pinned fingerprint: the
// server has no domain, so it has no publicly trusted certificate.
func (c *Client) httpClient() (*http.Client, error) {
	expected, err := base64.StdEncoding.DecodeString(c.Fingerprint)
	if err != nil || len(expected) != sha256.Size {
		return nil, fmt.Errorf("a impressão digital do token de conexão é inválida")
	}

	tlsConf := &tls.Config{
		// Replaced by VerifyPeerCertificate below, not skipped.
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("o servidor não apresentou certificado")
			}
			sum := sha256.Sum256(rawCerts[0])
			if !bytes.Equal(sum[:], expected) {
				return fmt.Errorf("o certificado do servidor não é o esperado")
			}
			return nil
		},
	}

	return &http.Client{
		Timeout:   httpTimeout,
		Transport: &http.Transport{TLSClientConfig: tlsConf},
	}, nil
}

func post(cli *http.Client, url string, payload, out any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := cli.Post(url, "application/json", bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("não consegui falar com o servidor de acesso: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		reason, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return &StatusError{StatusCode: resp.StatusCode, status: resp.Status, reason: strings.TrimSpace(string(reason))}
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxReplyBody)).Decode(out)
}

// StatusError is a non-200 answer from provisiond.
type StatusError struct {
	StatusCode int
	status     string
	reason     string
}

// IsRefusal reports whether err carries a 401 or 403: provisiond refused the
// token itself, as opposed to failing to answer.
func IsRefusal(err error) bool {
	var status *StatusError
	return errors.As(err, &status) &&
		(status.StatusCode == http.StatusUnauthorized || status.StatusCode == http.StatusForbidden)
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("o servidor recusou (%s): %s", e.status, e.reason)
}
