package supervisor

import (
	"net/http"
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
)

func leasingOptions() Options {
	return Options{Provision: profile.Provisioning{
		Server: "https://203.0.113.10:51821", Token: "vpn_test", Fingerprint: "fingerprint=",
	}}
}

var refusal = step{profileErr: &profile.StatusError{StatusCode: http.StatusUnauthorized}}

func lastAsk(h *harness) TokenAsk {
	return h.waits[len(h.waits)-1].status.TokenAsk
}

func TestRunAsksForAnotherTokenWhenTheServerRefusesIt(t *testing.T) {
	h := newHarness(t, leasingOptions(), refusal)
	h.run()

	if got := lastAsk(h); got != TokenRefused {
		t.Errorf("TokenAsk = %v, want TokenRefused on the first refusal", got)
	}
}

func TestRunAsksForAnotherTokenOnlyAfterGivingUpOnSilence(t *testing.T) {
	h := newHarness(t, leasingOptions(), fail, fail, fail)
	h.run()

	if got := h.waits[0].status.TokenAsk; got != TokenFine {
		t.Errorf("TokenAsk = %v after one failure, want TokenFine: a server may just be down", got)
	}
	if got := lastAsk(h); got != TokenUnanswered {
		t.Errorf("TokenAsk = %v once the retries were given up on, want TokenUnanswered", got)
	}
}

func TestRunNeverAsksForATokenWithAProfileFile(t *testing.T) {
	opts := leasingOptions()
	opts.ConfigPath = "/etc/wireguard/wg0.conf"
	h := newHarness(t, opts, refusal, fail, fail)
	h.run()

	for i, w := range h.waits {
		if w.status.TokenAsk != TokenFine {
			t.Errorf("attempt %d: TokenAsk = %v, want TokenFine: a .conf takes no token", i+1, w.status.TokenAsk)
		}
	}
}

func TestUseTokenLeasesWithItFromTheNextAttempt(t *testing.T) {
	sup := New(leasingOptions())
	sup.set(func(st *Status) { st.TokenAsk = TokenRefused })

	replacement := profile.Provisioning{Server: "https://203.0.113.20:51821", Token: "vpn_new", Fingerprint: "other="}
	sup.UseToken(replacement)

	if got := sup.currentProvision(); got != replacement {
		t.Errorf("provisioning = %+v, want the token just given", got)
	}
	if got := sup.Status().TokenAsk; got != TokenFine {
		t.Errorf("TokenAsk = %v, want TokenFine while the new token is tried", got)
	}
	select {
	case <-sup.nudge:
	default:
		t.Error("the new token did not cut the backoff short")
	}
}
