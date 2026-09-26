#!/bin/sh
# Checks the variables and profiles of both Compose files with "docker compose config",
# which needs neither a daemon nor a build. Skips when docker compose or jq is missing.
#
#   sh server/compose_test.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
failed=0

if ! docker compose version > /dev/null 2>&1 || ! command -v jq > /dev/null 2>&1; then
    echo "skip: docker compose or jq not available"
    exit 0
fi

# compose FILE NAME=value... [--] ARGS... runs docker compose on FILE with only the given
# variables set, ignoring any server/.env.
compose() {
    file=$1
    shift
    vars=
    while [ $# -gt 0 ] && [ "$1" != -- ]; do
        vars="$vars $1"
        shift
    done
    [ $# -gt 0 ] && shift
    # shellcheck disable=SC2086
    env -i PATH="$PATH" HOME="$HOME" $vars docker compose -f "$here/$file" --env-file /dev/null "$@"
}

check() {
    if [ "$2" = "$3" ]; then
        echo "ok: $1"
    else
        echo "FAIL: $1: got '$2', want '$3'"
        failed=1
    fi
}

# expect FILE SERVICE VAR WANT NAME=value... checks the value VAR gets in SERVICE's environment.
expect() {
    file=$1 service=$2 var=$3 want=$4
    shift 4
    got=$(compose "$file" "$@" -- --profile panel config --format json |
        jq -r --arg service "$service" --arg var "$var" '.services[$service].environment[$var] // "<unset>"')
    check "$file: $service $var with ${*:-nothing set}" "$got" "$want"
}

for file in docker-compose.yml docker-compose.host.yml; do
    expect "$file" provisiond PANEL 1
    expect "$file" provisiond PANEL 0 PANEL=0

    expect "$file" provisiond PANEL_ORIGIN "" PANEL_ORIGIN=
    expect "$file" provisiond PANEL_ORIGIN https://panel.example PANEL_ORIGIN=https://panel.example

    expect "$file" caddy PANEL_DOMAIN panel.example PANEL_DOMAIN=panel.example
done

image() {
    compose docker-compose.host.yml "$@" -- config --format json | jq -r '.services.provisiond.image'
}
check "docker-compose.host.yml runs the image of PROVISIOND_TAG" "$(image PROVISIOND_TAG=abc)" "ghcr.io/bumaruf/discord-go-vpn/provisiond:abc"
check "docker-compose.host.yml without PROVISIOND_TAG runs latest" "$(image)" "ghcr.io/bumaruf/discord-go-vpn/provisiond:latest"

services() {
    compose docker-compose.yml -- "$@" config --services | LC_ALL=C sort | tr '\n' ' '
}
check "docker-compose.yml without a profile leaves caddy out" "$(services)" "provisiond wireguard "
check "docker-compose.yml with profile panel runs caddy" "$(services --profile panel)" "caddy provisiond wireguard "

exit "$failed"
