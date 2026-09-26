#!/bin/sh
#
# Brings up the server interface inside the container. Peers are added by provisiond with "wg set".
set -eu

DIR=/etc/wireguard
# Same default as internal/provisiond/config.
WG_SUBNET=${WG_SUBNET:-10.9.0.0/24}
case $WG_SUBNET in
    *[!0-9./]* | *..* | */*/* | *.*.*.*.*) subnet_ok= ;;
    [0-9]*.[0-9]*.[0-9]*.0/24) subnet_ok=1 ;;
    *) subnet_ok= ;;
esac
if [ -z "$subnet_ok" ]; then
    echo "WG_SUBNET must be an IPv4 /24 such as 10.9.0.0/24, got $WG_SUBNET" >&2
    exit 1
fi

mkdir -p "$DIR"
chmod 700 "$DIR"

# Generated once: new keys would invalidate every client that holds this server's public key.
if [ ! -f "$DIR/server-private.key" ]; then
    echo "==> generating the server keys"
    umask 077
    wg genkey | tee "$DIR/server-private.key" | wg pubkey > "$DIR/server-public.key"
fi

# NAT on the wrong interface fails silently, so it comes from the default route, not "eth0".
NIC=$(ip -4 route ls default | awk '{print $5; exit}')
if [ -z "$NIC" ]; then
    echo "could not discover the outbound interface" >&2
    exit 1
fi

echo "==> outbound interface: $NIC"

# Private, CGNAT and link-local ranges; 169.254.169.254 is the cloud metadata service.
BLOCKED_DESTINATIONS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10,169.254.0.0/16

# Rewritten on every start: the peers live only in memory, as provisiond's temporary leases.
umask 077
cat > "$DIR/wg0.conf" <<EOF
[Interface]
Address = ${WG_SUBNET%.0/24}.1/24
ListenPort = 51820
PrivateKey = $(cat "$DIR/server-private.key")

# NAT, so tunnel traffic leaves with this machine's address. Anyone with an invite can build a
# full tunnel, so it may only reach the internet: the REJECTs, inserted last to sit above the
# ACCEPTs, keep it off the other peers, the private networks and the cloud metadata service.
PostUp = iptables -I FORWARD 1 -i %i -j ACCEPT; iptables -I FORWARD 1 -o %i -j ACCEPT; iptables -I FORWARD 1 -i %i -d ${BLOCKED_DESTINATIONS} -j REJECT; iptables -I FORWARD 1 -i %i -o %i -j REJECT; iptables -t nat -A POSTROUTING -o ${NIC} -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT; iptables -D FORWARD -o %i -j ACCEPT; iptables -D FORWARD -i %i -d ${BLOCKED_DESTINATIONS} -j REJECT; iptables -D FORWARD -i %i -o %i -j REJECT; iptables -t nat -D POSTROUTING -o ${NIC} -j MASQUERADE
EOF

# An orphan wg0 would make the next "wg-quick up" fail, so a stopped container takes it down.
trap 'wg-quick down wg0 >/dev/null 2>&1 || true; exit 0' TERM INT

wg-quick up wg0
wg show wg0

echo "==> wg0 up on port 51820/udp"

# wg-quick returns once the interface is up; the container needs a foreground process.
while wg show wg0 >/dev/null 2>&1; do
    sleep 30 &
    wait $!
done

echo "wg0 disappeared" >&2
exit 1
