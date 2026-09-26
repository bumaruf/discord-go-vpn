//go:build windows

// Package wiresock runs the tunnel on Windows with WireSock, whose WFP filter
// captures only Discord's processes.
package wiresock

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/platform/console"
	"github.com/bumaruf/discord-go-vpn/internal/platform/tasklist"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

const wiresockPackage = "NTKERNEL.WireSockVPNClientCLI"

type Tunnel struct {
	cmd  *exec.Cmd
	conf string

	// job holds wiresock-client; closing it kills the process.
	job windows.Handle
}

func Find() (string, error) {
	defaultPath := `C:\Program Files\WireSock Secure Connect\sdk\wiresock-client.exe`
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath, nil
	}

	if path, err := exec.LookPath("wiresock-client.exe"); err == nil {
		return path, nil
	}

	return "", errNotInstalled
}

var errNotInstalled = errors.New("o WireSock não está instalado")

// IsNotInstalled tells a missing WireSock, which Install fixes, from any other
// lookup failure.
func IsNotInstalled(err error) bool { return errors.Is(err, errNotInstalled) }

func Install() error {
	if _, err := exec.LookPath("winget"); err != nil {
		return fmt.Errorf("o WireSock não está instalado e o winget não existe nesta máquina.\n" +
			"  instale o WireSock manualmente: https://www.wiresock.net/\n" +
			"  (ele é o que filtra o túnel por aplicativo; sem ele a mídia do vídeo escapa)")
	}

	if output, err := winget(wiresockPackage); err != nil {
		return fmt.Errorf("não consegui instalar o WireSock: %w\n  %s\n"+
			"  tente à mão:  winget install %s",
			err, firstLine(output), wiresockPackage)
	}

	if _, err := Find(); err != nil {
		return fmt.Errorf("o winget terminou mas o wiresock-client.exe não apareceu.\n" +
			"  reinicie o computador e tente de novo")
	}
	return nil
}

func winget(id string) (string, error) {
	cmd := exec.Command("winget", "install", id,
		"--accept-package-agreements", "--accept-source-agreements", "--silent")
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.TrimSpace(text[:i])
	}
	return text
}

// writeConfig writes the file wiresock-client runs, which only reads its config
// from a file. Close removes it.
func writeConfig(cfg *wg.Config) (string, error) {
	path := filepath.Join(os.TempDir(), "discord-go-vpn.conf")
	// 0600: the file holds the private key.
	if err := os.WriteFile(path, []byte(renderConfig(cfg)), 0o600); err != nil {
		return "", fmt.Errorf("gravando %s: %w", path, err)
	}
	return path, nil
}

// Start runs wiresock-client in the foreground rather than as a service, so the
// filter dies with the process and an abnormal exit leaves no route behind.
// progress and warn carry what Start has to tell the person meanwhile.
func Start(cfg *wg.Config, progress, warn func(msg string)) (*Tunnel, error) {
	if !console.IsElevated() {
		return nil, fmt.Errorf("o WireSock instala um filtro no kernel e precisa de administrador.\n" +
			"  clique com o botão direito no .exe e escolha \"Executar como administrador\"")
	}

	exe, err := Find()
	if err != nil {
		if !IsNotInstalled(err) {
			return nil, err
		}
		progress("WireSock não encontrado, instalando (pode levar um ou dois minutos)…")
		if err := Install(); err != nil {
			return nil, err
		}
		progress("WireSock instalado")
		if exe, err = Find(); err != nil {
			return nil, err
		}
	}

	prepared, err := writeConfig(cfg)
	if err != nil {
		return nil, err
	}

	// A service left by an earlier run would keep its filter up.
	stopService()

	cmd := exec.Command(exe, "run", "-config", prepared, "-log-level", "info")
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(prepared)
		return nil, fmt.Errorf("subindo o WireSock: %w", err)
	}
	// Alive needs ProcessState, which only Wait sets.
	go func() { _ = cmd.Wait() }()

	t := &Tunnel{cmd: cmd, conf: prepared}

	// Windows does not end a child with its parent. Closing the console window
	// would leave the filter up with nobody renewing the lease, and Discord
	// would lose the network minutes later.
	if job, err := bindToProcess(cmd); err == nil {
		t.job = job
	} else {
		warn(fmt.Sprintf("não consegui amarrar o WireSock a este processo (%s); feche com Ctrl+C ou q, não pelo X da janela", errtext.Translate(err)))
	}

	return t, nil
}

// Alive reports whether wiresock-client still runs: Start only launches it, and
// it can exit right away (missing driver, rejected config, port in use).
func (t *Tunnel) Alive() error {
	if t.cmd == nil || t.cmd.Process == nil {
		return errors.New("o processo não chegou a ser criado")
	}

	if t.cmd.ProcessState != nil {
		return fmt.Errorf("encerrou logo após subir (%s)", t.cmd.ProcessState)
	}

	// Windows has no signal 0 to probe a process with.
	if !tasklist.Running("wiresock-client.exe") {
		return errors.New("o wiresock-client.exe não aparece na lista de processos")
	}
	return nil
}

// bindToProcess puts cmd in a KILL_ON_JOB_CLOSE Job Object, so it dies when the
// job handle closes, including when this process is killed.
func bindToProcess(cmd *exec.Cmd) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}

	// os.Process does not expose its handle.
	h, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(h) }()

	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func (t *Tunnel) Close() {
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
		t.cmd = nil
	}
	stopService()

	if t.conf != "" {
		_ = os.Remove(t.conf)
		t.conf = ""
	}

	if t.job != 0 {
		_ = windows.CloseHandle(t.job)
		t.job = 0
	}
}

func stopService() {
	_ = exec.Command("net.exe", "stop", "wiresock-client-service").Run()
	_ = exec.Command("taskkill.exe", "/F", "/IM", "wiresock-client.exe").Run()
}
