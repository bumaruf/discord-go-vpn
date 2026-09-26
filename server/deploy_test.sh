#!/bin/sh
# Runs deploy.sh from a temporary directory, with docker, curl and sleep stubbed on PATH.
# /health fails only while PROVISIOND_TAG is $bad.
#
#   sh server/deploy_test.sh [script]
set -eu

here=$(cd "$(dirname "$0")" && pwd)
script=${1:-$here/deploy.sh}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failed=0

good=1111111111111111111111111111111111111111
bad=2222222222222222222222222222222222222222
previous=3333333333333333333333333333333333333333

mkdir "$tmp/bin" "$tmp/server"
cp "$script" "$tmp/server/deploy.sh"
cat > "$tmp/bin/docker" <<STUB
#!/bin/sh
echo "\$*" >> "$tmp/docker"
STUB
cat > "$tmp/bin/curl" <<STUB
#!/bin/sh
! grep -q "^PROVISIOND_TAG=$bad\\\$" "$tmp/server/.env"
STUB
printf '#!/bin/sh\nexit 0\n' > "$tmp/bin/sleep"
chmod +x "$tmp/bin/docker" "$tmp/bin/curl" "$tmp/bin/sleep"

# run ENV SHA runs the script with .env holding ENV; its exit code goes to $status and the
# docker commands it ran to $tmp/docker.
run() {
    printf '%b' "$1" > "$tmp/server/.env"
    rm -f "$tmp/docker"
    status=0
    env -i PATH="$tmp/bin:$PATH" sh "$tmp/server/deploy.sh" "$2" > "$tmp/log" 2>&1 || status=$?
}

check() {
    if [ "$2" = "$3" ]; then
        echo "ok: $1"
    else
        echo "FAIL: $1: got '$2', want '$3'"
        failed=1
    fi
}

env_file() {
    tr '\n' ' ' < "$tmp/server/.env"
}

ups() {
    grep -c 'up -d provisiond' "$tmp/docker" 2>/dev/null || true
}

for arg in "" abc "$good; reboot" "$(echo "$good" | tr 1 A)" "${good}1"; do
    run 'SECRET=x\n' "$arg"
    check "rejects '$arg'" "$status" 2
    check "runs no docker command for '$arg'" "$(cat "$tmp/docker" 2>/dev/null)" ""
done

run 'SECRET=x\n' "$good"
check "a healthy deploy succeeds" "$status" 0
check "the first deploy appends the tag" "$(env_file)" "SECRET=x PROVISIOND_TAG=$good "
check "the image is pulled" "$(head -n 1 "$tmp/docker")" "pull -q ghcr.io/bumaruf/discord-go-vpn/provisiond:$good"
check "provisiond is recreated once" "$(ups)" 1

run 'SECRET=x' "$good"
check "the tag goes on its own line when .env lacks a final newline" "$(env_file)" "SECRET=x PROVISIOND_TAG=$good "

run "PROVISIOND_TAG=$previous\nSECRET=x\n" "$good"
check "a later deploy replaces the tag in place" "$(env_file)" "PROVISIOND_TAG=$good SECRET=x "

run "PROVISIOND_TAG=$previous\nSECRET=x\n" "$bad"
check "an unhealthy deploy fails" "$status" 1
check "an unhealthy deploy restores the previous tag" "$(env_file)" "PROVISIOND_TAG=$previous SECRET=x "
check "the rollback recreates provisiond again" "$(ups)" 2

run 'SECRET=x\n' "$bad"
check "an unhealthy first deploy fails" "$status" 1
check "an unhealthy first deploy has nothing to roll back to" "$(ups)" 1

exit "$failed"
