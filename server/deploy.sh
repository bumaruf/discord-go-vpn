#!/bin/sh
#
# Deploys one commit's provisiond image with docker-compose.host.yml, which sits next to this
# script, and rolls back to the previous image when the new one doesn't answer /health.
# It is the forced command of the CI's SSH key (server/README.md, Deploy contínuo), so the
# commit SHA is its only input and nothing runs before it is validated.
#
#   sudo ./deploy.sh <commit sha>
set -eu

IMAGE=ghcr.io/bumaruf/discord-go-vpn/provisiond
HEALTH_URL=https://localhost:51821/health
HEALTH_TRIES=30
KEEP_IMAGES=3

sha=${1:-}
case $sha in
    "" | *[!0-9a-f]*) sha= ;;
esac
if [ ${#sha} -ne 40 ]; then
    echo "usage: deploy.sh <40-character commit sha>" >&2
    exit 2
fi

cd "$(dirname "$0")"

compose() {
    docker compose -f docker-compose.host.yml "$@"
}

# .env holds secrets: these read and write only its PROVISIOND_TAG line.
current_tag() {
    sed -n 's/^PROVISIOND_TAG=//p' .env | tail -n 1
}

set_tag() {
    if grep -q '^PROVISIOND_TAG=' .env; then
        sed -i "s/^PROVISIOND_TAG=.*/PROVISIOND_TAG=$1/" .env
    else
        [ -z "$(tail -c 1 .env)" ] || echo >> .env
        echo "PROVISIOND_TAG=$1" >> .env
    fi
}

# The first tries only cover the startup, which waits for wg0 and Mongo.
healthy() {
    i=0
    while [ "$i" -lt "$HEALTH_TRIES" ]; do
        curl -fsSk --max-time 5 "$HEALTH_URL" > /dev/null 2>&1 && return 0
        i=$((i + 1))
        sleep 2
    done
    return 1
}

# Keeps the newest KEEP_IMAGES tags, so the previous image is still local for a rollback.
prune() {
    docker images "$IMAGE" --format '{{.Tag}}' | tail -n +$((KEEP_IMAGES + 1)) | while read -r tag; do
        docker rmi "$IMAGE:$tag" > /dev/null 2>&1 || true
    done
}

previous=$(current_tag)

echo "==> pulling $IMAGE:$sha"
docker pull -q "$IMAGE:$sha"
set_tag "$sha"
compose up -d provisiond

if healthy; then
    echo "==> $sha deployed"
    prune
    exit 0
fi

echo "deploy failed: $sha did not answer $HEALTH_URL" >&2
if [ -z "$previous" ]; then
    echo "no previous PROVISIOND_TAG in .env to roll back to; check: docker compose logs provisiond" >&2
    exit 1
fi
echo "==> rolling back to $previous" >&2
set_tag "$previous"
compose up -d provisiond
if ! healthy; then
    echo "the rollback to $previous is not healthy either; check: docker compose logs provisiond" >&2
fi
exit 1
