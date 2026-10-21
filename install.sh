#!/bin/sh
# Installs discord-go-vpn on Linux: a root systemd service that holds the tunnel,
# and a tray icon in the desktop session. Updates come through the client's own
# signed updater, which the service runs before each connection.
#
#   curl -fsSL https://raw.githubusercontent.com/bumaruf/discord-go-vpn/main/install.sh | sudo sh
#
# Options (after "sh -s --" when piped):
#   --user NAME      whose Discord the tunnel carries (default: who ran sudo)
#   --binary PATH    install this build instead of downloading the latest release
#   --uninstall      remove everything but the saved token
#   --purge          with --uninstall, the saved token too
set -eu

REPO=bumaruf/discord-go-vpn
PREFIX=/opt/discord-go-vpn
BIN=$PREFIX/discord-go-vpn
LINK=/usr/local/bin/discord-go-vpn
UNIT=/etc/systemd/system/discord-go-vpn.service
STATE=/var/lib/discord-go-vpn
APP_ENTRY=/usr/share/applications/discord-go-vpn.desktop
AUTOSTART=/etc/xdg/autostart/discord-go-vpn-tray.desktop
ICON=/usr/share/icons/hicolor/256x256/apps/discord-go-vpn.png

owner=${SUDO_USER:-}
binary=
action=install
purge=

say() { printf '  %s\n' "$*"; }
die() { printf '\n  erro: %s\n\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
	case $1 in
	--user) owner=${2:?--user precisa de um nome}; shift ;;
	--binary) binary=${2:?--binary precisa de um caminho}; shift ;;
	--uninstall) action=uninstall ;;
	--purge) purge=1 ;;
	*) die "opção desconhecida: $1" ;;
	esac
	shift
done

[ "$(id -u)" = 0 ] || die "rode como root: curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | sudo sh"
command -v systemctl >/dev/null || die "este instalador precisa do systemd"

uninstall() {
	systemctl disable --now discord-go-vpn.service 2>/dev/null || true
	rm -f "$UNIT" "$LINK" "$APP_ENTRY" "$AUTOSTART" "$ICON"
	rm -rf "$PREFIX"
	systemctl daemon-reload
	pkill -x -f "$BIN tray" 2>/dev/null || true
	if [ -n "$purge" ]; then
		rm -rf "$STATE"
		say "removido, inclusive o token salvo."
	else
		say "removido. O token salvo ficou em $STATE; --purge apaga."
	fi
}

if [ "$action" = uninstall ]; then
	uninstall
	exit 0
fi

[ -n "$owner" ] && [ "$owner" != root ] || die "não sei de quem é o Discord: rode com sudo a partir do seu usuário, ou passe --user NOME"
uid=$(id -u "$owner" 2>/dev/null) || die "o usuário $owner não existe"
home=$(getent passwd "$owner" | cut -d: -f6)

for cmd in ip sudo; do
	command -v "$cmd" >/dev/null || die "falta o comando $cmd (pacotes iproute2 e sudo)"
done

case $(uname -m) in
x86_64 | amd64) asset=discord-go-vpn ;;
aarch64 | arm64) asset=discord-go-vpn-arm64 ;;
*) die "arquitetura $(uname -m) sem build publicado" ;;
esac

echo
say "instalando o discord-go-vpn para $owner"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [ -n "$binary" ]; then
	cp "$binary" "$tmp/$asset"
else
	command -v curl >/dev/null || die "falta o comando curl"
	base=https://github.com/$REPO/releases/latest/download
	say "baixando a versão mais nova…"
	curl -fsSL -o "$tmp/$asset" "$base/$asset" || die "não consegui baixar $base/$asset"
	curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS" || die "não consegui baixar o SHA256SUMS da release"
	(cd "$tmp" && grep " $asset\$" SHA256SUMS | sha256sum -c --status) || die "o arquivo baixado não confere com o SHA256SUMS"
fi

# A run with sudo left the old tunnel up: two would fight over the same netns.
systemctl stop discord-go-vpn.service 2>/dev/null || true

install -d -m 0755 "$PREFIX"
install -m 0755 "$tmp/$asset" "$BIN"
ln -sf "$BIN" "$LINK"

# The service keeps the token where its XDG_CONFIG_HOME points. One saved by a
# run with sudo (in root's or the user's home) comes along.
install -d -m 0700 "$STATE" "$STATE/discord-go-vpn"
if [ ! -f "$STATE/discord-go-vpn/token" ]; then
	for old in /root/.config/discord-go-vpn/token "$home/.config/discord-go-vpn/token"; do
		if [ -f "$old" ]; then
			install -m 0600 "$old" "$STATE/discord-go-vpn/token"
			say "token salvo reaproveitado de $old"
			break
		fi
	done
fi

cat >"$UNIT" <<EOF
[Unit]
Description=discord-go-vpn: o Discord num túnel WireGuard
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$BIN service -user $owner
Environment=XDG_CONFIG_HOME=$STATE
StateDirectory=discord-go-vpn
StateDirectoryMode=0700
RuntimeDirectory=discord-go-vpn
RuntimeDirectoryMode=0755
Restart=on-failure
RestartSec=3
# The service opens Discord, which must outlive it: on stop only the service is
# signalled, and it hands Discord back to the normal network before exiting.
KillMode=process
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
EOF

install -d "$(dirname "$ICON")" "$(dirname "$APP_ENTRY")" "$(dirname "$AUTOSTART")"
"$BIN" icon "$ICON"

cat >"$APP_ENTRY" <<EOF
[Desktop Entry]
Type=Application
Name=discord-go-vpn
Comment=O Discord num túnel WireGuard só dele
Exec=$LINK tray
Icon=discord-go-vpn
Categories=Network;
Keywords=discord;vpn;wireguard;
EOF

cat >"$AUTOSTART" <<EOF
[Desktop Entry]
Type=Application
Name=discord-go-vpn
Comment=Ícone na bandeja; conecta o túnel ao entrar
Exec=$LINK tray
Icon=discord-go-vpn
X-GNOME-Autostart-enabled=true
EOF

command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q /usr/share/icons/hicolor 2>/dev/null || true

systemctl daemon-reload
systemctl enable --now discord-go-vpn.service >/dev/null
say "serviço ativo (systemctl status discord-go-vpn)"

# The tray belongs to the desktop session; its systemd user manager has the
# session's environment when the compositor exported it.
if [ -S "/run/user/$uid/bus" ] && systemd-run --user --machine="$owner@" --collect --quiet "$LINK" tray 2>/dev/null; then
	say "bandeja aberta"
else
	say "a bandeja abre no próximo login; para abrir agora: discord-go-vpn tray &"
fi

echo
say "pronto. A bandeja conecta o túnel; o painel abre por ela, ou com: discord-go-vpn"
say "sem token salvo, o painel pede o token de conexão (dgv1_…)."
echo
