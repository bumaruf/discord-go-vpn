# One image for both Compose services: provisiond runs "wg set" on the interface wireguard brings up.

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

# Same reproducible-build flags as the Makefile.
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-buildid=" -o /out/provisiond ./cmd/provisiond

FROM alpine:3.22

# wireguard-go is the fallback wg-quick uses when the host kernel has no WireGuard module.
RUN apk add --no-cache wireguard-tools wireguard-go iptables ip6tables

COPY --from=build /out/provisiond /usr/local/bin/provisiond
COPY server/wg-up.sh server/provisiond-up.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/wg-up.sh /usr/local/bin/provisiond-up.sh

# Server keys, tokens and certificate (server/README.md, Arquivos no volume).
VOLUME /etc/wireguard
