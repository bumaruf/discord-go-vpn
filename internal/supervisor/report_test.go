package supervisor

import (
	"io"
	"os"
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/transport"
)

func TestRunShowsTransportReportsInStatusNotTerminal(t *testing.T) {
	const (
		progress = "WireSock não encontrado, instalando (pode levar um ou dois minutos)…"
		warning  = "sem medição de tráfego: winget indisponível"
	)
	terminal := captureTerminal(t)
	h := newHarness(t, Options{},
		step{drop: true, report: func(r transport.Reporter) {
			r.Progress(progress)
			r.Warn(warning)
		}},
		step{})
	h.run()

	if got := terminal(); got != "" {
		t.Errorf("wrote to the terminal, which the TUI owns meanwhile: %q", got)
	}
	if len(h.starts) != 2 || len(h.probes) != 2 {
		t.Fatalf("got %d starts and %d probes, want 2 and 2", len(h.starts), len(h.probes))
	}
	if st := h.starts[0]; st.Progress != progress || st.Warning != warning {
		t.Errorf("while starting: {Progress: %q, Warning: %q}, want {%q, %q}", st.Progress, st.Warning, progress, warning)
	}
	if st := h.probes[0]; st.State != StateHealthy || st.Progress != "" || st.Warning != warning {
		t.Errorf("connected: {State: %v, Progress: %q, Warning: %q}, want {%v, \"\", %q}",
			st.State, st.Progress, st.Warning, StateHealthy, warning)
	}
	if st := h.starts[1]; st.Progress != "" || st.Warning != "" {
		t.Errorf("next connection: {Progress: %q, Warning: %q}, want both empty", st.Progress, st.Warning)
	}
}

// captureTerminal points os.Stdout and os.Stderr at a pipe until the returned
// func restores them and returns what was written.
func captureTerminal(t *testing.T) func() string {
	t.Helper()
	stdout, stderr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w

	written := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		written <- string(b)
	}()

	var got *string
	restore := func() string {
		if got == nil {
			os.Stdout, os.Stderr = stdout, stderr
			_ = w.Close()
			s := <-written
			got = &s
		}
		return *got
	}
	t.Cleanup(func() { restore() })
	return restore
}
