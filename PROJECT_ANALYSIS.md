# CleverConnect — Project Analysis

> Generated: 2026-06-10

## 1. Overview

**CleverConnect** is a self-hosted, multi-protocol **VPN / proxy orchestrator and download hub** written in Go, with two embedded React single-page applications (a *client* panel and a *server* panel). A single binary can boot in either **client** or **server** mode (selected via `APP_MODE`), serving the matching embedded UI and enabling mode-specific subsystems.

The project bundles an unusually broad feature set under one roof:

- **Proxy cores**: V2Ray, Xray, and sing-box integration (config compilation, subscription parsing, speed/latency testing, CDN scanning).
- **Tunneling**: Ehco relay tunnels and a custom **Soroush WebRTC ("Hive")** tunnel built on MTProto + LiveKit.
- **Download hub**: HTTP/leech downloader, BitTorrent client, YouTube and Spotify downloaders.
- **Network tooling**: DNS resolver tester (UDP/TCP/DoT/DoH/DoQ), domain reachability checker, GeoIP/CDN intelligence engine.
- **Automation & ops**: enterprise job scheduler (cron), Telegram bot bridge, file manager, structured logging, system stats, WebDAV.

It targets deployment on **Clever Cloud** (see `.clever.json`) and ships a multi-stage `Dockerfile`.

### Scale at a glance

| Layer | Size |
|---|---|
| Go backend | ~43,700 LOC across 19 `internal/` modules |
| Client SPA | ~32,100 LOC (TS/TSX/SCSS), 22 pages |
| Server SPA | ~18,100 LOC, 17 pages |
| Largest backend module | `handlers/` (~10,400 LOC, 21 files) |
| Proxy subsystem | `v2ray/` (~8,900 LOC, 27 files, 9 sub-packages) |

---

## 2. Architecture

```
                       ┌─────────────────────────────┐
                       │   Single Go binary (Gin)    │
                       │   APP_MODE = client|server  │
                       └──────────────┬──────────────┘
                                      │
          ┌───────────────────────────┼───────────────────────────┐
          │                           │                           │
   Embedded SPA (go:embed)     REST API /api/*            WebSocket /ws/*
   client/dist or server/dist  (JWT-protected)           (logs, stats, jobs,
                                                           v2ray tests, scanner)
                                      │
        ┌─────────────┬──────────────┼───────────────┬─────────────┐
        │             │              │               │             │
     Proxy cores   Tunnels       Download hub     Net tooling   Automation
   (v2ray/xray/   (ehco,        (leech/torrent/  (dns/domain/   (scheduler/
    sing-box)      soroush)      youtube/spotify)  geo)          telegram)
                                      │
                          ┌───────────┴───────────┐
                          │   Persistence layer    │
                          │  GORM → SQLite(client) │
                          │       / MySQL(server)  │
                          │  + PebbleDB (v2ray     │
                          │    node configs)       │
                          └────────────────────────┘
```

### Key design patterns

- **Dual-mode single binary** — `config.LoadConfig()` reads `APP_MODE`; `main.go` conditionally initializes server-only engines (downloader, torrent, youtube, spotify, scheduler) and selects which embedded SPA to serve.
- **Database-driven runtime config** — most subsystems persist their settings in the DB and auto-start on boot if an "active" config row exists (Ehco, Telegram, Soroush, V2Ray inbounds).
- **Singleton engines** with `sync.Once` (DNS, domain checker, torrent, scheduler, geo).
- **Observer / listener** pattern for streaming results (geo lookups, domain checks) out over WebSockets.
- **Worker pools** via goroutines + channels for concurrent scanning, checking, and downloading.
- **Graceful DB fallback** — server mode prefers MySQL but falls back to a local SQLite file.
- **Embedded assets** — both SPAs are compiled and embedded via `//go:embed`, with an SPA-fallback static handler in `main.go`.

---

## 3. Backend modules (`internal/`)

| Module | LOC | Purpose |
|---|---|---|
| `config` | 149 | Loads `.env` (godotenv); resolves mode, port, JWT, DB creds, admin seed. |
| `db` (+ sqlite, pebble) | 2,081 | GORM layer; SQLite (client) / MySQL (server); PebbleDB KV store for V2Ray node configs + SQLite→Pebble migration. |
| `dns` | 1,911 | DNS resolver tester: multi-protocol (UDP/TCP/DoT/DoH/DoQ), censorship & DNSSEC diagnostics, rate limiting. |
| `domainchecker` | 294 | Concurrent domain reachability checker (DNS/HTTP/TLS/latency), worker pool. |
| `downloader` | 766 | Multi-threaded HTTP downloader with resume, proxy, and RealDebrid premium support. |
| `ehcocore` | 371 | Ehco relay tunnel client/server management (SNI masking, mux, dynamic bridge). |
| `filecore` | 1,198 | File ops + TAR/ZIP archive create/extract with streaming. |
| `geo` | 1,918 | GeoIP/CDN intelligence: IP2Location + MaxMind + QQWry via radix/Patricia trie for fast CIDR lookups. |
| `handlers` | 10,432 | All HTTP REST + WebSocket handlers (21 files). Largest: `v2ray.go` (~62KB), `dns.go` (~37KB), `ws.go` (~26KB). |
| `logger` | 593 | Async structured logging + Gin middleware/recovery. |
| `models` | 743 | GORM schema (users, sessions, tunnel/job/config models). |
| `scheduler` | 1,036 | Cron-based enterprise scheduler (robfig/cron): priority queue, workers, retries, timeouts. |
| `soroush` | 1,528 | Soroush WebRTC tunnel engine: MTProto handshake, LiveKit SFU conn, SOCKS5, relay, token mgmt. |
| `soroushlib` | 3,675 | Low-level MTProto: sessions, TL serialization, AES-IGE crypto, group-call protocol, DC conn. |
| `spotify` | 1,703 | Spotify metadata + download pipeline. |
| `telegram` | 3,931 | Telegram bot framework: commands, file bridge, MTProto auth, job notifications. |
| `torrent` | 608 | BitTorrent client (anacrolix/torrent): trackers, DHT/PEX, speed limiting. |
| `v2ray` | 8,864 | Proxy core suite — see breakdown below. |
| `youtube` | 837 | YouTube downloader (yt-dlp/ffmpeg) with format/quality selection. |

### `v2ray/` sub-packages

- `compiler/` — converts stored config into V2Ray/Xray JSON (VMess/VLESS/Trojan/SS, routing).
- `core/` — starts/stops the proxy binary, talks to its stats gRPC API.
- `scanner/` — port/CDN scanner to discover working endpoints; live telemetry over WS.
- `tester/`, `speed/` — benchmark profiles (latency, throughput via SOCKS5).
- `sub/` — subscription URL parser + background auto-updater (client mode).
- `traffic/` — real-time per-user traffic accounting and interceptor.
- `sysproxy/` — OS-level system proxy toggling (Win/Linux/macOS).
- `deeplink/` — share-link parsing.

### `core/` (runtime binaries & assets)

Holds the external proxy engine binaries and geo databases: `v2ray`/`v2ctl`, `xray`, `sing-box` (+ `libcronet.so`), plus `geoip.dat` / `geosite.dat` and JSON config templates.

---

## 4. Frontend (`web/client` & `web/server`)

Both apps share an **identical stack**, diverging only in feature pages:

- **React 19 + TypeScript 6 + Vite 8**, **React Router 7**, **Tailwind CSS 4** + Sass.
- **Zustand** stores for state; **localStorage** for the JWT.
- Build: `tsc -b && vite build`; vendor code-splitting; package manager is **Bun**.
- A global `fetch` interceptor catches `401` and redirects to `/login`.
- Real-time log/stat streaming over WebSocket.
- Testing: **Vitest** (unit) + **Playwright** (e2e), light coverage, V2Ray-focused.

| | Client | Server |
|---|---|---|
| Pages | 22 | 17 |
| Zustand stores | 9 | 6 |
| Emphasis | Network tools, scanners, subscriptions, maps (Leaflet), virtual scrolling | Server admin, traffic audits, user quotas |

**Client pages** include: Dashboard, V2Ray (Client/Core/Routing/Dashboard), DNS Tester, Domain/IP checkers, Network Tools, Ehco, Soroush, Leech, Torrent, YouTube, Spotify, Files, Player, Job Scheduler, Telegram Settings, Logs, Settings, Login.

---

## 5. Build, deploy & tooling

- **Makefile** — `install` (go mod + bun), `build` (frontend then backend), `run-client`/`run-server`, `test`, `lint` (`go fmt` + `go vet`), `setup-husky`. Also builds a separate `ehco` binary.
- **Dockerfile** — multi-stage: Bun builds the server SPA → Go 1.26.3 builds binaries → final runtime image.
- **CI** — `.github/workflows/build.yml`.
- **Git hooks** — Husky `pre-push` runs `make lint && make test`.
- **Deployment target** — Clever Cloud (`.clever.json`); `nginx.conf`, `mediamtx.yml`, `start.sh` present.

### Notable dependencies

`gin`, `gorm` (+mysql / modernc sqlite), `cockroachdb/pebble`, `xtls/xray-core`, `sagernet/sing-box`, `Ehco1996/ehco`, `pion/webrtc/v4`, `livekit/server-sdk-go`, `gotd/td` & `telebot.v4` (Telegram), `anacrolix/torrent`, `quic-go`, `kkdai/youtube`, `gopsutil`, `getsentry/sentry-go`, `golang-jwt`.

---

## 6. Security notes

- **Auth**: JWT bearer middleware guards all `/api` routes (except login & public subscription `/sub/:token`) and WebSockets (via query-param token).
- **Committed secrets / concerns** to address before any production use:
  - `.env.example` ships a weak default `JWT_SECRET` and hard-coded admin creds (`salman`/`136517`) used as the seed — these should be rotated and never committed.
  - The repo root contains large committed artifacts: two ~234 MB binaries (`clever-connect`, `main`), plus `ips.txt/csv/clean.txt` scan outputs — these bloat the repo and likely don't belong in VCS.
- **Dual-use nature**: includes port/CDN scanners, censorship-circumvention tunnels, and proxy cores. Intended for self-hosted personal/anti-censorship use; operate only on infrastructure you own or are authorized to test.

---

## 7. Observations & suggestions

**Strengths**
- Cohesive single-binary architecture with clean mode separation and embedded UIs.
- Very broad, well-modularized feature surface; consistent engine/singleton patterns.
- Strong real-time story (WebSocket telemetry across scanner, logs, stats, traffic).

**Risks / opportunities**
- **No top-level README** — onboarding relies on reading `main.go` and the Makefile.
- **Repo hygiene** — remove committed binaries and scan-output text files; add them to `.gitignore`.
- **Secrets** — move admin seed + JWT secret entirely to runtime env; document rotation.
- **Test coverage** — backend tests exist only for a few modules (downloader, filecore, soroush crypto, spotify api); the large `handlers` and `v2ray` packages are largely untested.
- **`handlers/v2ray.go` (~62KB)** is a god-file; could be split by concern (inbounds, client configs, scanner, testing).
- **Branch sprawl** — feature branches (`feat/v2ray`, `feat/telegram-core`, `feat/soroush-webrtc-vpn`, `feat/integerate-ehco`) suggest active parallel development; current checkout is `feat/v2ray`.

---

*This document was generated from a structural exploration of the codebase (entry point, module layout, dependencies, build tooling). It does not reflect runtime behavior or a line-by-line audit.*
