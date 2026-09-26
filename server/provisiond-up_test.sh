#!/bin/sh
# Runs provisiond-up.sh against a temporary volume, with wg, wget and provisiond stubbed on PATH.
#
#   sh server/provisiond-up_test.sh [script]
set -eu

here=$(cd "$(dirname "$0")" && pwd)
script=${1:-$here/provisiond-up.sh}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failed=0

mkdir "$tmp/bin"
printf '#!/bin/sh\nexit 0\n' > "$tmp/bin/wg"
printf '#!/bin/sh\necho 198.51.100.1\n' > "$tmp/bin/wget"
cat > "$tmp/bin/provisiond" <<STUB
#!/bin/sh
echo "panel=\$VPN_PANEL_SECRET" > "$tmp/received"
echo "\$*" > "$tmp/args"
STUB
chmod +x "$tmp/bin/wg" "$tmp/bin/wget" "$tmp/bin/provisiond"

# run VOLUME [NAME=value...] runs the script with its volume at VOLUME and prints what
# provisiond received; the script's exit code goes to $status, its output to $tmp/log and
# provisiond's arguments to $tmp/args.
run() {
    volume=$1
    shift
    mkdir -p "$volume"
    sed "s|^DIR=/etc/wireguard\$|DIR=$volume|" "$script" > "$tmp/up.sh"
    if ! grep -q "^DIR=$volume\$" "$tmp/up.sh"; then
        echo "FAIL: no DIR=/etc/wireguard line to redirect in $script"
        exit 1
    fi
    rm -f "$tmp/received" "$tmp/args"
    status=0
    env -i PATH="$tmp/bin:$PATH" ENDPOINT=203.0.113.1:51820 "$@" sh "$tmp/up.sh" > "$tmp/log" 2>&1 || status=$?
    cat "$tmp/received" 2>/dev/null || true
}

check() {
    if [ "$2" = "$3" ]; then
        echo "ok: $1"
    else
        echo "FAIL: $1: got '$2', want '$3'"
        failed=1
    fi
}

secret() {
    cat "$1" 2>/dev/null || echo "<missing>"
}

generated() {
    value=$(secret "$1")
    if [ ${#value} -eq 44 ] && [ -n "$(find "$1" -perm 600)" ]; then
        echo yes
    else
        echo "no: '$value'"
    fi
}

files() {
    LC_ALL=C ls "$1" | tr '\n' ' '
}

generation_log() {
    grep '^==> generating' "$tmp/log" | tr '\n' ' '
}

v=$tmp/fresh
out=$(run "$v")
check "fresh volume logs the generated secret" "$(generation_log)" "==> generating VPN_PANEL_SECRET "
check "fresh volume generates panel-actions.token" "$(generated "$v/panel-actions.token")" yes
check "fresh volume gets only the token file" "$(files "$v")" "panel-actions.token "
check "provisiond receives the generated secret" "$out" "panel=$(secret "$v/panel-actions.token")"
check "a second start keeps the secret" "$(run "$v")" "$out"
check "a second start generates nothing" "$(generation_log)" ""

v=$tmp/env
out=$(run "$v" VPN_PANEL_SECRET=panel-from-env)
check "the environment wins over the volume" "$out" "panel=panel-from-env"
check "no file is generated for a secret set in the environment" "$(ls "$v")" ""

v=$tmp/no-panel
out=$(run "$v" PANEL=0)
check "PANEL=0 generates nothing" "$(ls "$v")" ""
check "PANEL=0 leaves the panel secret empty" "$out" "panel="

run "$tmp/ports" ENDPOINT= > /dev/null
check "default ports" "$(cat "$tmp/args")" "-interface wg0 -listen :51821 -endpoint 198.51.100.1:51820"

v=$tmp/no-random
mkdir "$tmp/broken"
printf '#!/bin/sh\nexit 1\n' > "$tmp/broken/head"
chmod +x "$tmp/broken/head"
run "$v" PATH="$tmp/broken:$tmp/bin:$PATH" > "$tmp/out"
check "a failed generation stops the script" "$([ "$status" -ne 0 ] && echo stopped || echo "exit $status")" stopped
check "a failed generation does not start provisiond" "$(cat "$tmp/out")" ""
check "a failed generation saves no token" "$(secret "$v/panel-actions.token")" "<missing>"

v=$tmp/empty-file
mkdir -p "$v"
(umask 077 && : > "$v/panel-actions.token")
out=$(run "$v")
check "an empty token file is generated again" "$(generated "$v/panel-actions.token")" yes
check "provisiond receives the regenerated token" "$out" "panel=$(secret "$v/panel-actions.token")"

exit "$failed"
