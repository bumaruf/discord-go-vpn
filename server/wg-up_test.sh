#!/bin/sh
# Runs wg-up.sh against a temporary volume, with wg, wg-quick and ip stubbed on PATH, and checks
# the wg0.conf it writes.
#
#   sh server/wg-up_test.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failed=0

mkdir "$tmp/bin"
# The first "wg show" succeeds and the next fails, which ends the script's wait loop.
cat > "$tmp/bin/wg" <<STUB
#!/bin/sh
case \$1 in
genkey) echo private-key ;;
pubkey) cat > /dev/null; echo public-key ;;
show) [ ! -e "$tmp/shown" ] && : > "$tmp/shown" ;;
esac
STUB
printf '#!/bin/sh\nexit 0\n' > "$tmp/bin/wg-quick"
printf '#!/bin/sh\necho "default via 192.0.2.1 dev eth0"\n' > "$tmp/bin/ip"
chmod +x "$tmp/bin/wg" "$tmp/bin/wg-quick" "$tmp/bin/ip"

# conf NAME=value... runs the script with a fresh volume and prints the Address and ListenPort
# lines of the wg0.conf it wrote.
conf() {
    volume=$(mktemp -d "$tmp/volume.XXXXXX")
    sed "s|^DIR=/etc/wireguard\$|DIR=$volume|" "$here/wg-up.sh" > "$tmp/wg-up.sh"
    rm -f "$tmp/shown"
    env -i PATH="$tmp/bin:$PATH" "$@" sh "$tmp/wg-up.sh" > "$tmp/log" 2>&1 || true
    grep -E '^(Address|ListenPort) =' "$volume/wg0.conf" 2>/dev/null | tr '\n' ' '
}

check() {
    if [ "$2" = "$3" ]; then
        echo "ok: $1"
    else
        echo "FAIL: $1: got '$2', want '$3'"
        failed=1
    fi
}

check "defaults" "$(conf)" "Address = 10.9.0.1/24 ListenPort = 51820 "

# The REJECTs must come after the ACCEPTs, since each "-I FORWARD 1" lands above the previous one.
conf > /dev/null
postup=$(grep '^PostUp =' "$volume/wg0.conf")
check "PostUp blocks the metadata service and private networks" \
    "$(echo "$postup" | grep -c -- '-I FORWARD 1 -o %i -j ACCEPT; iptables -I FORWARD 1 -i %i -d 10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10,169.254.0.0/16 -j REJECT; iptables -I FORWARD 1 -i %i -o %i -j REJECT;')" "1"
check "WG_SUBNET sets the address" "$(conf WG_SUBNET=10.8.0.0/24)" "Address = 10.8.0.1/24 ListenPort = 51820 "

exit "$failed"
