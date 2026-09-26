#!/bin/sh
#
# Prepares provisiond's environment and execs it.
set -eu

DIR=/etc/wireguard
PANEL=${PANEL:-1}

# Secrets left empty in the environment are generated once and kept in the volume, since
# changing one invalidates whoever already holds it.
#
# secret_from_volume VAR FILE keeps VAR when set, otherwise loads it from FILE. An empty file can
# only come from a failed generation, so when FILE holds no secret, one is generated into it.
secret_from_volume() {
    eval "[ -n \"\${$1:-}\" ]" && return
    file=$DIR/$2
    secret=
    if [ -f "$file" ]; then
        secret=$(cat "$file")
        [ -n "$secret" ] || echo "warning: $file is empty" >&2
    fi
    if [ -z "$secret" ]; then
        echo "==> generating $1"
        umask 077
        # The pipeline's status is tr's, so its output is checked instead: 32 random bytes are
        # 44 base64 characters.
        secret=$(head -c 32 /dev/urandom | base64 | tr -d '\n')
        if [ ${#secret} -ne 44 ]; then
            echo "could not generate $file" >&2
            exit 1
        fi
        printf '%s' "$secret" > "$file"
    fi
    eval "$1=\$secret"
}

if [ "$PANEL" = "1" ]; then
    secret_from_volume VPN_PANEL_SECRET panel-actions.token
fi
VPN_PANEL_SECRET=${VPN_PANEL_SECRET:-}
export VPN_PANEL_SECRET

# Behind NAT (Oracle, for one) the NIC address is private, so an external service is asked.
if [ -z "${ENDPOINT:-}" ]; then
    IP=$(wget -qO- --timeout=10 https://api.ipify.org 2>/dev/null || true)
    if [ -z "$IP" ]; then
        echo "could not discover the public IP; set ENDPOINT in .env" >&2
        exit 1
    fi
    ENDPOINT="${IP}:51820"
fi

echo "==> endpoint given to clients: ${ENDPOINT}"

# depends_on waits for the container, not for wg0 inside it, and provisiond exits without it.
i=0
while ! wg show wg0 >/dev/null 2>&1; do
    i=$((i + 1))
    if [ "$i" -gt 60 ]; then
        echo "wg0 did not appear within 60s" >&2
        exit 1
    fi
    sleep 1
done

exec provisiond -interface wg0 -listen :51821 -endpoint "$ENDPOINT"
