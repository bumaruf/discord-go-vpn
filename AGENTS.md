# AGENTS.md

Guidance for coding agents in this repository. The READMEs are the reference for how things work and how to run them; this file holds what they don't: the owner's decisions, invariants the code relies on, and gotchas.

## Decisions vs. current state

The code grew faster than its conventions. Only the rules under **Decisions** were made by the owner. Everything else — naming, language, patterns, and the rationale written in `panel/README.md` and code comments — is current state, not precedent. Keep it working, but don't extend it as a standard; when a choice isn't covered below, ask.

## Decisions

- **English in code, Brazilian Portuguese for people.** Identifiers, comments, commit messages, database collections, fields and stored values, HTTP routes, env vars, and output only operators read (logs, flag help, script and Makefile output) are in English. Strings the end user reads — client TUI and CLI messages, panel UI — are in Brazilian Portuguese, keeping established technical terms (download, upload, peer, RTT, handshake); stored values are translated when the panel renders them.
- **Markdown docs are in Brazilian Portuguese** — READMEs and `docs/` — so the team reads them easily. Agent instruction files (`AGENTS.md`, `CLAUDE.md`, `panel/AGENTS.md`) are in English.
- **Domain terms have one English name each**, listed in `docs/glossary.md`. Name new code from it.
- **Renames keep the old name working.** When a route, JSON key, ticket field, env var, flag, file on disk or in the volume, or Mongo collection, field or value gets a new name, the code accepts both and the pair goes into `docs/legacy.md` in the same PR, creating the file if it doesn't exist. Legacy is removed in one pass, when the owner judges every user has migrated; until then Mongo keeps its old names.
- **DRY and Clean Code** are the standards for architecture and code organization.
- **Comments only where they are necessary.** A comment earns its place by saying what the code can't: why a surprising choice was made, a non-obvious constraint or gotcha, a contract with another component. Clear code stays uncommented. Keep each comment short and each fact in one place — not restated at every call site, in the package doc and in the README.
- **Release tags are semver versions** (`v1.2.3`), never dates: clients update only to a higher semver, so a date tag would outrank every later version.
- **Commits:** `type(scope): description`, in English. Scopes: `client`, `android`, `panel`, `provisiond`, `server`, `tray`, `docs`, `contracts` (Go and TypeScript changed together), `ci`.

## Where things are

Three deliverables talking over the network; the layout is in `README.md` → *Estrutura do repositório*, the architecture diagrams in `README.md` → *Arquitetura*.

- **Client** (`cmd/discord-go-vpn` + `internal/`): runs the whole Discord process inside a WireGuard tunnel (a network namespace on Linux, the WireSock WFP filter on Windows), so gateway and media leave from the same IP. The **Android app** (`android/` + `mobile/`) is the same client on a phone.
- **`provisiond`** (`cmd/provisiond` + `internal/provisiond/`): runs on the VPS through `server/`. Hands out short-lived WireGuard peer leases, collects metrics, serves the live SSE and the admin routes, and is the panel's only way to the database. Reference: `server/README.md` → *Referência* (routes, env vars, volume files, timings and limits).
- **Panel** (`panel/`): Next.js 16 on a Cloudflare Worker (`@opennextjs/cloudflare`), admin-only, at `vpn.bumaruf.dev`. Reference: `panel/README.md`. Its `panel/AGENTS.md` applies there.

## Checks

The `Makefile` header lists its targets; the panel's are its `package.json` scripts. CI (`.github/workflows/ci.yml`) runs `make test`, `make lint`, `make android` and the panel's `lint`, `typecheck`, `format:check`, `test` and `build`: keep them green before pushing. A push to `main` deploys what changed: provisiond's image to the VPS (`deploy-server.yml`), the panel to Cloudflare (`deploy-panel.yml`); a pull request touching `panel/` gets a Workers Preview.

- The gofmt check in `make test` skips `.claude/worktrees/` and `panel/`.
- The `GOOS=windows` vet is what catches a broken `_windows.go` from Linux.
- CI sets no secrets, so the integration tests skip there. Each one's command is in its file comment:
  - `internal/provisiond/auth/auth_integration_test.go` and `internal/provisiond/store/panel_integration_test.go`: `MONGODB_INTEGRATION=1` + `MONGODB_URI`. They drop `invites` in `vpn_auth_test` and the whole `vpn_panel_test` on the cluster the URI points to.
  - `internal/provisiond/auth/interop_test.go`: `TICKET_INTEROP` + `TICKET_SECRET`.

## Client

Flow: `main.go` → `app.Run` → `startup.Run` (`update.Install`, then the `.conf` or `connectiontoken.Ensure`) → `supervisor.Run` → on each connection, `profile.Open` + `transport.New`. The lease that validated the connection token is handed to the supervisor's first connection; reconnections lease afresh. Startup runs inside the screen, started from its `Init`, so a terminal that can't draw falls back to plain text before anything ran; `startup.Reporter` is implemented by both. A release `update.Install` wrote is relaunched only after the screen closes.

- **`ui`** is Bubble Tea v2. It reads `Status` from `Updates()` and polls `Supervisor.Reading()` (traffic, RTT) once a second; only transports implementing `transport.Meter` (netns, vpnservice) report traffic, never WireSock.

- **`supervisor`** owns the tunnel. The UI and the tray only read `Status` snapshots from `Updates()`, which drops frames instead of blocking. A dropped session is closed (transport, then lease) before the next attempt, because both transports tear down by fixed name; backoff 3s→30s; after 3 consecutive failures `StateFallback` hands Discord back to the normal network and keeps retrying. Every connection that comes up reopens Discord inside it; on exit Discord reopens outside. A lease gone for good (`profile.ErrLeaseLost`: a 410, or failed renewals spanning its TTL) ends the session like a drop.
- **`transport`** is the `Transport` interface with its adapters (`netns_linux.go`, `wiresock_windows.go`); the implementations live in `transport/netns` and `transport/wiresock`. Transports never write to the terminal, which the TUI owns: `Start` reports through `transport.Reporter` into `Status.Progress` and `Status.Warning`.
- **`profile`**: a `.conf` in the folder wins; otherwise a lease from provisiond with a fresh key pair each run (in memory on Linux; through a temp file WireSock reads on Windows), renewed every 60s, released on `Close`, TLS pinned to the self-signed certificate's fingerprint. Teardown: transport before lease. `profile/profiletest` is the fake provisiond the tests share.
- **Connection token** (`dgv1_` + unpadded base64url of `{server, fingerprint, invite}`): binaries embed nothing. `Parse` rejects malformed tokens locally; only a 401/403 on the validation lease discards the saved token, while no answer keeps it.
- **`update`**: a release build trusts only the ed25519 `publicKey` compiled in, replaces itself and relaunches (`VPN_UPDATE_RELAUNCHED` stops a loop); `dev` never updates; any failure only warns. `cmd/updatesign sign` refuses a key that doesn't match `publicKey`.
- **User-facing errors** go through `errtext.Translate` before a person reads them.
- **Per-platform files**: a `_linux.go`/`_windows.go` function called from untagged code needs an `_other.go` stub. Packages imported only from tagged files (`transport/netns`, `transport/wiresock`, `platform/tray`) have none.
- Windows: the `.exe` requests elevation through `build/app.manifest`, compiled into the committed `rsrc_windows_*.syso`; a build without it relaunches itself elevated (`console.EnsureElevated`).

## Android

- `mobile/` is the gomobile-bound API: `Connect`/`ConnectFile` start the desktop's `supervisor` with `internal/transport/vpnservice` and a no-op `RestartDiscord`. gomobile binds only basic types: lists cross as comma-separated strings, errors carry already-translated text. `gobind -lang=java ./mobile` checks a signature change without the NDK.
- `android/` only draws and owns the VPN: `TunnelService` implements the Go `Host` and is the only writer of `Tunnel`. The VPN allows only `com.discord`, so the app's own sockets stay outside it. The theme copies `panel/app/globals.css` tokens; keep them in step.
- `android` satisfies the `linux` build tag: a `_linux.go` file that must stay off Android needs `&& !android`.

## Panel

- **The panel never connects to Mongo.** Pages and Server Actions use the repositories in `panel/lib/` (`invites.ts`, `devices.ts`, `usage.ts`, `audit.ts`), which call provisiond's panel data routes through `readProvisiond`/`writeProvisiond` (`lib/provisiond.ts`, `Bearer VPN_PANEL_SECRET`). provisiond holds the only pool of Atlas connections: a Worker can't keep one between requests, and a Durable Object doesn't run near the cluster (sa-east-1). Only the Node scripts (`npm run setup`, `merge-devices`) use the driver.
- Failed logins are counted per address in the `LOGIN_ATTEMPTS` Workers KV namespace, failing open.
- `lib/env.ts` is the only reader of `process.env` outside tests: an empty value counts as unset. OpenNext copies the Worker's env into `process.env` once per isolate, so a changed secret may need a code deploy to show up.
- Nothing in the Worker may load a native `.node` module (hence argon2 from `@noble/hashes`). `cloudflare-env.d.ts` is generated from `wrangler.jsonc` (gitignored); `typecheck` and `build` generate it first.
- Admin API routes, Server Actions and the dashboard layout check `requireAdmin()`.
- Styling is Tailwind v4 with shadcn/ui (dark only): theme tokens in `app/globals.css`, shadcn components in `components/ui` (`npx shadcn add <name>`). A test fake used by more than one test is `lib/fake-*.ts`.

## provisiond

- `cmd/provisiond/main.go` only parses flags and wires `internal/provisiond/`. `config` is the only env reader; `store/names.go` holds every Mongo name (the panel's `lib/mongo-names.ts` must match the names it uses); shared test fakes live in `wireguardtest` and `authtest`.
- **Leases live in memory.** On SIGTERM they are saved to `leases.json` and the next start adopts the unexpired ones whose peer is still on wg0, so a deploy reconnects nobody; `metrics.Collector.Resume` keeps their usage from counting twice. Timings and the orphan-peer case: `server/README.md` → *Tempos e limites*.
- **Every access is an invite.** Every lease registers a device keyed by invite + label + `os_user`, ignoring case; when the key finds nothing, the lease adopts the same machine's device stored without an OS user or under another invite, and moves it to this invite, so its usage history stays whole. The unique index behind the key is created by the panel's `npm run setup`, and its collation must match `deviceCollation` in `store`.
- **Mongo is where invites are checked**, not only metrics: provisiond exits at startup without the database, because a 401 makes clients discard their saved token while a server that is down doesn't. Later, an invite in the 60s cache keeps working with Atlas down; an uncached one gets a 503, which isn't cached.
- The collector reads `wg show dump` every 1s, feeds the SSE hub, and queues writes on `store.Writer`, which sends them from its own goroutine so a slow Atlas never delays the tick. Live data never goes through the database.
- Shutdown order (`service.run`): HTTP server, then the collector's unsaved usage, then the `store.Writer` flush, then Mongo; the Compose `stop_grace_period` must cover those timeouts.
- Admin route groups answer 404 to any method while their own secret isn't configured, checking the secret before their mux matches the method. `PANEL=0` only stops `server/provisiond-up.sh` from generating `VPN_PANEL_SECRET`.

## Contracts between Go and TypeScript

These break only in production, silently: change one side, change the other, and run the interop or integration test.

- **Invite hash**: `panel/lib/token.ts` (SHA-256) ↔ `internal/provisiond/auth/auth.go`.
- **Connection token**: `panel/lib/connection-token.ts` (from `PROVISIOND_LEASE_URL` and `PROVISIOND_FINGERPRINT`) ↔ `internal/connectiontoken/token.go`.
- **SSE ticket**: `panel/lib/ticket.ts` ↔ `ValidateTicketAt` in `internal/provisiond/auth/ticket.go` (`{uid, role, exp}`, unpadded base64url JSON + HMAC-SHA256 with `VPN_LIVE_SECRET`).
- `VPN_PANEL_SECRET` and `VPN_LIVE_SECRET` hold the same value in the panel's Worker secrets and on the server.
- **Wire shapes** (lease request and response, admin action response, the panel data routes, SSE snapshot, ticket, connection token) are fixtures in `testdata/contracts/`, checked against the Go types by `internal/contracttest` and against the TypeScript types by `panel/lib/contracts.test.ts` (enforced by `npm run typecheck`). A wire change updates the fixture, and both sides follow.
