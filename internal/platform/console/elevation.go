package console

import (
	"fmt"
	"os"
)

// EnsureElevated returns when the process already runs as administrator.
// Otherwise it relaunches the program elevated and exits: 0 once the elevated
// copy is running, 1 after telling the person how to elevate by hand.
func EnsureElevated() {
	if IsElevated() {
		return
	}
	fmt.Println("pedindo privilégio de administrador (o WireSock instala um filtro no sistema)…")
	if Elevate() {
		os.Exit(0)
	}
	fmt.Fprint(os.Stderr, "\nnão consegui pedir a elevação automaticamente.\n"+
		"  clique com o botão direito no programa e escolha \"Executar como administrador\"\n")
	HoldWindow()
	os.Exit(1)
}
