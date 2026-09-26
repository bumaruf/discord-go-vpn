# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Decisions vs. current state

The code grew faster than its conventions. Only the rules under **Decisions** were made by the owner. Everything else — naming, language, patterns, and the rationale written in `panel/README.md` and code comments — is current state, not precedent. Keep it working, but don't extend it as a standard; when a choice isn't covered below, ask.

## Decisions

- **English in code, Brazilian Portuguese for people.** Identifiers, comments, commit messages, database collections, fields and stored values, HTTP routes, env vars, and output only operators read (logs, flag help, script and Makefile output) are in English. Strings the end user reads — client TUI and CLI messages, panel UI — are in Brazilian Portuguese, keeping established technical terms (download, upload, peer, RTT, handshake); stored values are translated when the panel renders them.
- **Markdown docs are in Brazilian Portuguese** — READMEs and `docs/` — so the team reads them easily. Agent instruction files (`CLAUDE.md`, `panel/AGENTS.md`) are in English.
- **Domain terms have one English name each**, listed in `docs/glossary.md`. Name new code from it.
- **Renames keep the old name working.** When a route, JSON key, ticket field, env var, flag, file on disk or in the volume, or Mongo collection, field or value gets a new name, the code accepts both and the pair goes into `docs/legacy.md` in the same PR, creating the file if it doesn't exist. Legacy is removed in one pass, when the owner judges every user has migrated; until then Mongo keeps its old names.
- **DRY and Clean Code** are the standards for architecture and code organization.
- **Comments only where they are necessary.** A comment earns its place by saying what the code can't: why a surprising choice was made, a non-obvious constraint or gotcha, a contract with another component. Clear code stays uncommented. Keep each comment short and each fact in one place — not restated at every call site, in the package doc and in the README.
- **Commits:** `type(scope): description`, in English. Scopes: `client`, `panel`, `provisiond`, `server`, `tray`, `docs`, `contracts` (Go and TypeScript changed together), `ci`.

## What this is

Three deliverables in one repository, talking over the network:

- **Client** (`cmd/discord-go-vpn` + `internal/`) — runs the whole Discord process inside a WireGuard tunnel: a network namespace on Linux, the WireSock WFP filter on Windows, so gateway and media (WebRTC over UDP) leave from the same IP.
- **`provisiond`** (`cmd/provisiond` + `internal/provisiond/`) — runs on the VPS via `server/` (Docker Compose). Hands out short-lived WireGuard peer leases (`wg set`), collects metrics, serves live SSE and the admin routes.
- **Panel** (`panel/`) — Next.js 16 on Vercel + MongoDB Atlas, admin-only. It has its own `panel/AGENTS.md`: this Next version differs from what the model knows; read `node_modules/next/dist/docs/` before writing code there.

Reference docs, so this file doesn't duplicate them:
- `README.md` → *Arquitetura*: diagrams of the whole system and of one client session.
- `server/README.md` → *Referência*: `provisiond` routes and auth, env vars, files in the volume, timings and limits.
- `panel/README.md`: panel env vars, local setup, Vercel deploy.

## Commands

CI (`.github/workflows/ci.yml`, on pull requests and pushes to `main`) runs `make test`, `make lint` and the panel's `npm ci`, `lint`, `typecheck`, `format:check`, `test` and `build` — keep them all green locally before pushing. It sets no secrets, so integration tests skip there too. A push to `main` that changes provisiond's image then calls `deploy-server.yml`, which publishes it to GHCR and runs `server/deploy.sh` on the VPS (`server/README.md` → *Deploy contínuo*).

Go (repo root; `go.mod` has `ignore ./panel` so `./...` stays out of `node_modules`):

```sh
make                 # client for this OS (embeds nothing; access comes from a connection token or a .conf)
make dist            # linux/amd64, windows/amd64, windows/arm64 + SHA256SUMS (reproducible build)
make provisiond      # linux server binary into dist/
make test            # fails on unformatted Go files, go vet (Linux and GOOS=windows), go test ./... (internal and cmd), the server/*_test.sh scripts (compose_test.sh skips without docker compose or jq)
make syso            # regenerate Windows resources (manifest, version info, icon); the .syso files are committed
make lint            # golangci-lint v2.13.2 (pinned, via go run) on Linux and GOOS=windows; config in .golangci.yml

go test ./internal/wg -run TestName -count=1   # single test
```

`make test` gotchas:
- The gofmt check skips `.claude/worktrees/` and `panel/`; fix a failure with `gofmt -w <file>`.
- The `GOOS=windows` vet is what catches a broken `_windows.go` from Linux.

Integration tests skip themselves without their env vars; the exact command is in each file's comment:
- `internal/provisiond/auth/auth_integration_test.go` — `MONGODB_INTEGRATION=1` + `MONGODB_URI` (real Mongo; drops `invites` in database `vpn_auth_test`).
- `internal/provisiond/auth/interop_test.go` — `TICKET_INTEROP` + `TICKET_SECRET` (ticket signed by the panel).

Panel (`panel/`): `npm run dev`, `npm run lint` (fails on warnings), `npm run typecheck`, `npm run format:check` (`npm run format` to fix), `npm test` (Vitest, `*.test.ts` next to the file under test), `npm run build`, `npm run setup` (collections and indexes, idempotent), `npm run hash-password -- "password"` (base64 argon2 hash for `.env.local`).

## Client architecture

Flow: `main.go` (flags and wiring) → without a `.conf`, `connectiontoken.Ensure` (prompt, validation, saved file) → `app.Run` → `supervisor.Run` → on each connection, `profile.Open` + `transport.New(mode, reporter)`. Startup leases once: the lease that validated the connection token (or `app.Run`'s first `profile.Open`) is handed along and used by the supervisor's first connection; reconnections lease afresh.

Packages in `internal/`: `connectiontoken`, `app` (runs the TUI, or plain text when it can't draw, and the tray), `supervisor`, `profile`, `transport`, `routing` (the routing modes), `wg` (`.conf` parsing), `discord` (finding, closing and reopening Discord), `ui`, `errtext`, `platform/{console,tray,tasklist,osuser}` (`osuser` names the OS user sent with a lease); `contracttest` checks `testdata/contracts/`, and `profile/profiletest` is the fake `provisiond` shared by tests.

- **`supervisor`** owns the tunnel and its lifecycle. The UI (`ui`, Bubble Tea, or plain text) and the tray only read `Status` snapshots from `Updates()`, which drops frames instead of blocking. Policy: one `Transport` serves the whole run, so Discord is never restarted between connections; a dropped session is closed (tunnel via `Stop`, then lease) before the next attempt, because both transports tear down by fixed name; backoff 3s→30s; after 3 consecutive failures it enters `StateFallback`, `Leave` points Discord at the normal network and it keeps retrying. Every connection that comes up calls `Enter`, which points Discord at it. A failed lease renewal lands in `Status.LastError`; once the lease is gone (a 410, or failed renewals spanning its TTL: `profile.ErrLeaseLost`) the session ends like a drop. Transports never write to the terminal, which the TUI owns: `Start` reports setup progress and warnings through `transport.Reporter`, kept in `Status.Progress` (cleared when the attempt ends) and `Status.Warning` (cleared by the next attempt). On exit `Close` leaves Discord on the normal network.
- **`profile`** picks the config source: a `.conf` in the folder wins; otherwise a lease from `provisiond` (`lease.go`) — a fresh key pair each run (in memory on Linux; on Windows it goes through a temp file WireSock reads), `/lease` (hostname as `label`, `osuser.Name()` as `os_user`) → `/renew` every 60s → `/release` on `Close`, TLS pinned to the self-signed certificate's fingerprint. Teardown order: transport before lease.
- **Connection token** (`internal/connectiontoken`): binaries embed nothing, so a lease needs `dgv1_` + unpadded base64url of `{server, fingerprint, invite}` — from `-token`, `VPN_TOKEN`, the file saved under `os.UserConfigDir()/discord-go-vpn/token`, or the terminal prompt. `Parse` rejects malformed tokens locally; only a 401/403 on the validation lease discards the saved token, while no answer keeps it and starts anyway.
- **`transport`** has two layers: the `Transport` interface + adapters (`transport/netns_linux.go`, `transport/wiresock_windows.go`), and the implementations in `transport/netns` (TUN created outside and moved into the namespace; wireguard-go encrypts from the original namespace) and `transport/wiresock` (installs WireSock via winget; a Job Object ties the child process to ours). Discord's window and the focus are never touched: a Discord that loaded inside the tunnel is only made to reconnect by ending its TCP connections (`ss -K` inside the namespace on Linux, `discord.DropConnections` on Windows); one that loaded outside is reopened minimized (`discord.MinimizedFlag`), since Discord reads the IP's location only on a fresh load and in Brazil it blocks screen sharing and the camera. On Linux the namespace outlives each connection and the fallback is a veth pair NATed by the host; on Windows the transport remembers the Discord it opened by `discord.StartedAt`.
- **User-facing errors** go through `errtext.Translate` (`internal/errtext`) before a person reads them (TUI, plain text, transport warnings built from an error): it swaps the English that Go, the OS and commands add for pt-BR by error type, keeping the client's own Portuguese context.
- **Per-platform files**: a `_linux.go`/`_windows.go` function called from untagged code needs an `_other.go` stub, or other OS builds break (`transport`, `discord`, `app`, `platform/console`). Packages imported only from tagged files (`transport/netns`, `transport/wiresock`, `platform/tray`) have none; `GOOS=windows go vet ./...` and the other OS builds are the check.
- Windows: the `.exe` requests elevation through `build/app.manifest`, compiled into `rsrc_windows_*.syso`; a build without it relaunches itself elevated in wiresock mode (`console.EnsureElevated`).

## Panel architecture

- Pages, API routes and Server Actions never query Mongo: they go through the repositories in `panel/lib/` (`invites.ts`, `devices.ts`, `sessions.ts`, `samples.ts`, `usage.ts`, `audit.ts`), which return English records. Every collection, field and stored value name is in `lib/mongo-names.ts`, the counterpart of `store/names.go`.
- `lib/env.ts` is the only reader of `process.env` outside tests: an empty value counts as unset.
- Admin API routes, Server Actions and the dashboard layout check `requireAdmin()` (`lib/require-admin.ts`); API routes answer its 401 without a session and 403 without the admin role.
- Styling is Tailwind v4 with shadcn/ui (dark only): the theme tokens live in `app/globals.css`, and `components/ui` holds the shadcn components (`npx shadcn add <name>`) next to the panel's own. Charts use Recharts through `components/ui/chart.tsx`. A test fake used by more than one test is `lib/fake-*.ts`, imported only by tests.

## `provisiond` and the contracts with the panel

- **Layout**: `cmd/provisiond/main.go` only parses flags and wires the packages in `internal/provisiond/`: `config` (the only env reader), `wireguard` (the `wg` CLI and dump parser), `lease` (registry, IP pool, reaper), `store` (Mongo; every collection, field and stored value name is in `store/names.go`), `auth` (invites, SSE ticket, bearer), `metrics` (collector), `live` (SSE hub and handler), `httpapi` (routes and handlers, where the HTTP tests live) and `tlscert`. Test fakes shared across packages live in `wireguardtest` and `authtest`.
- Leases live in memory: 180s TTL, renewed by the client every 60s, expired by a `reaper` every 30s, so a client that dies silently is gone in ~3.5 min. On SIGTERM they are saved to `leases.json` in the volume (`lease/snapshot.go`), and the next start adopts those still unexpired whose peer is still on wg0 with the same address, so a deploy reconnects nobody; `metrics.Collector.Resume` keeps their usage from being counted twice. A restart without that file (a crash) leaves the peers of connected clients on wg0 without a lease: their next renewal gets a 410 and they reconnect, new leases skip their addresses, and the reaper removes such an orphan once its last handshake is more than 15 min old.
- **Every access is an invite** (kind `individual` or `global`, which only the panel reads). Every lease registers a device keyed by invite + label + `os_user`; when that key finds nothing, the lease adopts the same machine's device stored without an OS user or with a null invite, and the same write brings it up to date, so its usage history (grouped by device id) stays whole; `npm run merge-devices` merges a machine that already has two devices. The unique index behind the key is created by the panel's `npm run setup`.
- **Mongo is where devices are registered and invites are checked**, not only metrics. provisiond exits at startup without `MONGODB_URI` or when the database can't be opened, since a 401 would make clients discard their saved token while a server that is down doesn't. With Atlas down later, an invite already in the 60s cache in `auth` keeps working; an uncached one gets 503 on `/lease`, `/renew` and `/release`, which isn't cached, so it gets in as soon as Atlas answers. The SSE keeps running.
- The collector (`metrics`) reads `wg show dump` every 1s (handling counter resets when a peer is recreated), feeds the SSE hub (`live`), and every 5s/30s enqueues writes on `store.Writer`, which sends them to Atlas from its own goroutine so a slow Atlas never delays the tick. Live data never goes through the database.
- On SIGTERM or SIGINT, `service.run` (`cmd/provisiond/main.go`) shuts the HTTP server down (live streams end through `Hub.Close`; other requests finish, and connections left after the timeout are closed), waits for the collector to queue its unsaved usage, then flushes `store.Writer` and closes Mongo; the Compose `stop_grace_period` must cover its timeouts.
- Admin routes answer 404 to any method when their own secret isn't configured: in `httpapi`, each route group checks its secret before its own mux matches the method, and checks the bearer or ticket after; `/admin/live` needs only `VPN_LIVE_SECRET`. `PANEL=0` only stops `server/provisiond-up.sh` from generating `VPN_PANEL_SECRET` — it disables nothing set explicitly.

TypeScript ↔ Go contracts that only break in production, silently — change one side, change the other and run the interop/integration test:
- **Invite hash**: `panel/lib/token.ts` (SHA-256) ↔ `internal/provisiond/auth/auth.go`.
- **Connection token**: `panel/lib/connection-token.ts` (built from `PROVISIOND_LEASE_URL` and `PROVISIOND_FINGERPRINT`) ↔ `internal/connectiontoken/token.go`.
- **SSE ticket**: `panel/lib/ticket.ts` ↔ `ValidateTicketAt` in `internal/provisiond/auth/ticket.go` (`{uid, role, exp}` as unpadded base64url JSON + HMAC-SHA256 with `VPN_LIVE_SECRET`).
- `VPN_PANEL_SECRET` and `VPN_LIVE_SECRET` must hold the same value on Vercel and on the server.

The wire shapes of the lease request and response, the admin action response, the SSE snapshot, the ticket payload and the connection token payload live in `testdata/contracts/`. `go test` checks them against the Go types (`internal/contracttest`), and the panel against its TypeScript types (`panel/lib/contracts.test.ts`, enforced by `npm run typecheck`). A wire change updates the fixture, and both sides must follow.
