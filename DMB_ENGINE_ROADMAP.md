# CleverConnect — Dynamic Multipath Bonding (DMB) Engine Roadmap

> Status: design roadmap (not yet implemented)
> Scope: a **new, additive** V2Ray connection engine ("Turbo / Multipath") that consumes scanned & worked nodes from PebbleDB as its source of *lines (arteries)*. It does **not** replace the existing single-line V2Ray core path — that stays exactly as is.
> Grounded against the codebase at commit on branch `feat/v2ray`. File/line references are from the current tree.

---

## Part 0 — Read this first: four corrections that change the design

Your blueprint is ~85% sound and the hardest prerequisite (a continuous CDN/clean-IP scanner feeding PebbleDB) already exists. But an adversarial review of the load-bearing claims surfaced four issues that **must** shape the build. Ignoring any one of them produces an engine that corrupts streams, leaks keys, or is unusable on a real Iranian uplink.

### Correction 1 — True bonding requires a **single shared egress (combiner)**. The diversity is in the *path to the combiner*, not the internet egress. ✅ (your scanner already assumes this)

Packet racing/striping with server-side dedup/reorder keeps **one** connection-level sequence space and **one** reassembly buffer across all paths. That state can only live at one terminating endpoint. You therefore **cannot** packet-bond across N independent public V2Ray nodes that each egress to the internet from their own IP — a single logical TLS flow to `youtube.com` is pinned to one 5-tuple and one TLS secret and cannot leave from two different egress IPs.

So there are **two distinct modes**, and the roadmap keeps them separate:

| | **Mode A — Selector / Failover** | **Mode B — True Bonding (DMB)** |
|---|---|---|
| Egress | Each node egresses independently | **One** combiner egress (Clever Cloud origin) |
| Granularity | Whole TCP/QUIC flow pinned to one node (sticky) | Sub-packet: frames raced/striped across paths |
| Needs a server combiner? | **No** | **Yes** |
| Gives bandwidth aggregation? | No (only across many flows) | Yes (striping) |
| Works with arbitrary scanned nodes? | **Yes** | Only nodes that front/chain to the *same* combiner |
| Build cost | Low (Xray balancer + a Go control loop) | High (new framed transport + combiner + scheduler) |

Your scanner (`internal/v2ray/scanner/`) already finds **clean CDN-edge IPs** (Cloudflare/CloudFront/Fastly/Akamai ranges in `data/cdn_ips/`) — the classic "clean IP" technique. Those edges front a single origin. That is exactly the input Mode B needs, **provided every artery in a bonding group fronts the same combiner origin**. The roadmap requires the scanner to tag origin identity and refuse to mix origins in one bonding group.

### Correction 2 — The proposed frame header and PSK crypto are unsafe. ❌

* `[16B SessionID][8B Seq][2B len][payload]` is **insufficient**: it carries no frame **type** and no **destination**. The combiner terminates all arteries, so it cannot know the real egress `host:port` unless the protocol carries it. Your own code proves this — `internal/soroush/relay.go` + `socks5.go` read a `[2B len][host:port]` header before dialing. Without `OPEN`/`CLOSE`/`RST` control frames you also can't free per-stream dedup/reorder state → unbounded memory leak.
* **ChaCha20-Poly1305 with a global PSK + a `(SessionID,Seq)` nonce is catastrophically broken.** A repeated `(key,nonce)` leaks the keystream *and* the Poly1305 one-time key (plaintext disclosure **and** forgery). The two directions reusing the same `Seq` space under one PSK is a guaranteed collision.
* The inner AEAD is also **largely redundant** — each artery is already a TLS 1.3 / QUIC tunnel.

**Decision:** default to **no inner AEAD** (rely on each artery's TLS 1.3). If an end-to-end layer is genuinely wanted, derive a per-session key via HKDF over the PSK + a fresh handshake salt, split into **two directional keys**, and use a **per-direction monotonic counter** as the nonce. See [§3](#part-3--the-corrected-wire-protocol).

### Correction 3 — Duplication ≠ bandwidth aggregation. They are two different modes; don't promise both from one mechanism. ❌

* **Redundancy (duplication / racing):** broadcast identical frames on all active paths, dedup by `(StreamID,Seq)`. Cuts loss & tail latency. **Multiplies upload by N** — it does *not* add bandwidth.
* **Bonding (striping):** send each distinct frame on one chosen path, weighted by measured per-path RTT/throughput. **Sums bandwidth.** Needs reorder + retransmit-on-path-death.

On a throttled, asymmetric **Iranian residential uplink, duplication beyond N=1 is unusable** for anything but keystrokes. **Striping is the default; duplication is opt-in** as FEC/hedge on a small fraction of frames.

### Correction 4 — Clever Cloud constrains the server combiner to **WebSocket-over-HTTPS on :8080**. No UDP, no QUIC to origin, no arbitrary listeners. ❌→✅

Clever Cloud puts every app behind the **Sozu** reverse proxy: one HTTP port (**8080**), no UDP passthrough. **QUIC/HTTP3 direct to the origin is impossible.** Your repo already proves the working shape: `internal/ehcocore/engine.go` dials `wss://…:8080/tunnel`, and `nginx.conf` listens only on 8080. So the combiner is **inbound-only WebSocket (or gRPC-over-H2) on :8080**; arteries connect *up* to it. (Raw TCP is possible only via Clever's TCP-redirection add-on; WS makes it unnecessary.)

> **Net effect on your plan:** keep the user-facing SOCKS5 `:10646` / HTTP `:10545` listeners and the per-artery local inbounds — those are correct. Change the *server transport* from "QUIC/gRPC on 8080" to **WS-over-HTTPS on 8080**, make **striping the default**, shrink on-wire frames to **2–4 KB**, add **control frames + safe crypto**, and require **single-origin bonding groups**.

---

## Part 1 — Strategy: ship Mode A first, then Mode B ("Turbo")

Your written goals split cleanly across the two modes:

* *"ping all lines always", "switch all traffic to the fastest line", "monitor lines, fast-switch on problems", "retire a line on errors and acquire a new config", "smart error thresholds", "no switching trap"* → **Mode A (Selector/Failover).** Delivers ~80% of the *felt* benefit (fast, reliable, self-healing, no dropped sockets) and needs **zero server changes**. The Xray compiler already emits a **balancer + observatory** (`BalancerConfig{leastPing|random}`, `compiler.go:701-711`) — most of Mode A is configuration + a Go control loop.
* *"king ways in parallel", "bond/aggregate", "racing", "sum bandwidth"* → **Mode B (DMB bonding).** The moonshot. Needs the combiner, the framed transport, and the scheduler.

**Recommended sequencing:** build Mode A end-to-end (engine + page + telemetry) → prove the bonding transport on a **loopback harness** → stand up the combiner on Clever Cloud → ship Mode B as a "Turbo" toggle on the same page. Each milestone below is independently shippable and never touches the existing single-line flow.

---

## Part 2 — What already exists and is reused (grounding)

### Source of lines — Scanner + PebbleDB (`internal/v2ray/scanner/`, `internal/db/pebble/db.go`)

* **Node schema is the canonical artery record:** `models.V2RayClientConfig` (`models.go:468-488`) — `Protocol, Address, Port, UUID, Network, TLSSettings(JSON: security/sni/publicKey/shortId/path), LatencyMs, PacketLoss, DownloadSpeedMBps, CdnProvider, PopLocation, Priority, IsActive`.
* **"Top-N fastest" query already exists:** `pebble.ListClientConfigs(filter ConfigFilter, offset, limit)` with `filter.SortBy="latency"` (`db.go:232,290-306`). The engine pulls its candidate pool with `limit=N`.
* **Throughput probe is reusable:** `scanner.testProxyThroughput()` (`telemetry_helpers.go:173-289`) spins an ephemeral **in-process sing-box** SOCKS5 and measures TTFB + MB/s. Reuse for warm-probing shadow paths.
* **CDN identity is reusable:** `CDNRegistry.Lookup(ip)` + `probeCdnPop()` to tag provider/POP and (critically for Mode B) to verify front-ability.
* **Failover building blocks:** `SaveLastSettings()/LoadLastSettings()` (pebble key `cache:last_scan_config`, `scanner.go:959-981`) and `reloadClientCore()` (`scanner.go:1450`).

### Core management (`internal/v2ray/core/`, `internal/v2ray/traffic/`, `internal/v2ray/compiler/`)

* **Cores run as external subprocesses** (`core.go` `StartCore` → `exec.CommandContext(bin,"run","-c",cfg)`). Default core = **xray** (`GetSelectedCoreName`, DB key `v2ray_core`).
* **xray-core is imported only as a gRPC *client*** for `StatsService` on `127.0.0.1:10085` (`interceptor.go:18,98`). **`HandlerService` is not yet enabled** — `api.services` lists only `StatsService`/`LoggerService` (`compiler.go:294,637`).
* **The compiler already supports multi-inbound / multi-outbound + routing** (`RoutingRule{InboundTag[],OutboundTag,BalancerTag}` `compiler.go:201-210`; balancer+observatory `:701-711`). It already uses a **`dokodemo-door` fixed-dest inbound** for the API (`:316,622`) — exactly the pattern the artery inbounds need.
* Reload today = **full process restart** (`ReloadCoreConfig`→`StartCore`), which drops all connections. Mode B needs **per-artery hot-swap** instead (see [§6 / M5](#m5--xray-hot-swap-handlerservice)).

### Server orchestration & transport (`internal/ehcocore/engine.go`, `internal/soroush/`)

* `ehcocore` proves the **client-dials-`wss://…:8080/tunnel`** shape and the single-remote topology — the template for the combiner transport.
* `soroush/relay.go` proves the **per-stream `[len][host:port]` + bidirectional `io.Copy`** relay and a QUIC-over-WebRTC path (`engine.go runQuicServer`, PMTUD off, 1200-byte pad) — confirms on-wire frames must be small.
* **Engine lifecycle idiom** (singleton + `sync.Mutex` + `context.CancelFunc`, `StartEngine/StopEngine/Status`, auto-start from an `IsActive` DB row in `main.go:100-161`) is the template for the bonding engine's lifecycle.

### Frontend (`web/client/src/`)

* **Page/route idiom:** `App.tsx:8-26,123-154` (lazy import + route object + breadcrumb `Record`) + `Sidebar.tsx` nav entry.
* **Zustand + live WS store:** `dashboardStore.ts:106-426` (`connectStream()` → `WS /ws/stats?token=…` → `set()`), and the advanced-test WS (`V2RayClientPage.tsx:105-218`, `WS /ws/v2ray/test`).
* **Virtualized results table to clone:** `pages/v2ray-client/components/SubscriptionsCard.tsx:1271-1424` (`@tanstack/react-virtual`, sticky head, multi-select, double-click-to-test).
* **Design atoms:** `Card`, `Button`, `Badge` (pulse), `Input`, `SplineChart` (dual-line live chart, `atoms/SplineChart.tsx`), `IPResolveBadge`. **No Toggle atom yet** — add one.
* **Auth:** `Bearer` from `localStorage('cc_client_token')`; global 401 interceptor in `main.tsx`.

---

## Part 3 — The corrected wire protocol

> Used **only** in Mode B. Mode A needs none of this.

### 3.1 Framing (`internal/bonding/frame`)

```
Frame on the wire (per artery, after the artery's own TLS):
┌────────┬────────┬───────────┬──────────┬──────────┬───────────────┐
│ Ver(1) │ Type(1)│ StreamID(4)│ Seq(8)   │ Len(2)   │ Payload(Len)  │
└────────┴────────┴───────────┴──────────┴──────────┴───────────────┘

Type:
  0 OPEN   payload = "host:port" (reuse relay.go's [len][host:port] convention)
  1 DATA   payload = stream bytes
  2 FIN    half-close this StreamID
  3 RST    abort this StreamID (with reason byte)
  4 PING    keepalive / RTT probe (carries echo nonce + send-timestamp)
  5 WINDOW  flow-control credit update (carries new window)
```

* **`StreamID` (4B, monotonic per direction, low-bit = initiator parity)** identifies a user connection — far cheaper than a 16-byte per-frame SessionID. A single random **16-byte bonded-session id** is exchanged once at handshake, not per frame.
* **`Seq` is connection-level**, independent of any single path → a path is just a numbered-frame transport; this is what makes seamless failover possible.
* Two **independent** Seq spaces (c2s, s2c).

### 3.2 Crypto

* **Default: no inner AEAD.** Each artery already runs TLS 1.3 (VLESS/REALITY/WS). Don't re-encrypt; it only adds CPU and a nonce footgun.
* **Optional end-to-end mode (defense-in-depth across heterogeneous arteries):**
  * Per bonded session: `K = HKDF-Expand(PSK, handshake_salt(32B random) ‖ session_id)`.
  * Directional keys: `K_c2s = HKDF(K,"c2s")`, `K_s2c = HKDF(K,"s2c")`.
  * Nonce (96-bit) = `32-bit handshake prefix ‖ 64-bit per-direction counter`, **never reused, never reset**; rekey/teardown before counter wrap.
  * Frame header goes in **AEAD associated data** (authenticated, not encrypted).
  * Broadcast copies of the *same* frame across N arteries share a nonce **by design** (identical ciphertext, not a re-encryption) — safe.

### 3.3 Flow control & reorder (`internal/bonding/session`)

* **Per-stream credit window (yamux/QUIC-style):** receiver advertises a window; the splitter stops reading the source app when outstanding-unacked bytes hit it; `WINDOW` frames replenish. Plus a **global per-session reorder-buffer cap**. This is the backpressure the original design lacked — without it a stalled slow path bloats memory for every session.
* **Reorder exploits that each artery is reliable & in-order.** Track `next-expected Seq` per stream + a bounded out-of-order buffer. **A gap is not loss** — the frame is still in flight on a slower (live) artery. So: **no fixed wall-clock loss timeout.** Track **per-artery liveness** (PING/keepalive); only when an artery is declared dead do its unacked Seqs get **re-sent over surviving arteries**. This requires the splitter to remember in-flight Seq→artery assignments until acked.

---

## Part 4 — Topology & deployment

### 4.1 The only two topologies that work for Mode B

* **(A) CDN-fronted single origin** — xray `VLESS+WS+TLS` to many clean Cloudflare/ArvanCloud edge IPs that all front-route to the **one** Clever Cloud `:8080` WS combiner. *Cheap; reuses the scanner directly. But the CDN is a shared egress and a single point of failure — the N-path diversity is partly illusory.*
* **(B) Relay-VPS fan-in** — N independent relay VPS (diverse ASNs) each hold a persistent WS/gRPC tunnel **up** to the Clever `:8080` combiner; the client stripes across relays. *Real path diversity; more infra to run.*

**Recommendation:** support both. Start with **(A)** because it drops straight onto the scanner; document **(B)** as the high-resilience option. **A bonding group must never mix origins.**

### 4.2 Server combiner (Clever Cloud)

* Listener: **WebSocket-over-HTTPS on TCP `:8080`**, behind Sozu (mirror `ehcocore` + `nginx.conf`). **No UDP/QUIC to origin.**
* **Inbound-only:** arteries connect up; the combiner writes responses back down the same sockets. It never dials edges.
* **Single-instance, sticky.** Dedup/reorder state is in-memory; if Clever scales/redeploys, state scatters and reassembly breaks. Pin to one instance (or add a shared coordinator later — out of scope for v1).
* **Mandatory keepalives** (PING frames) to survive Cloudflare (~100 s idle) and Sozu idle timeouts under Iranian throttling.

### 4.3 Ports (canonical)

| Port | Role | Bind |
|---|---|---|
| `10646` | **User-facing SOCKS5** (Mode A & B frontend) | `127.0.0.1` |
| `10545` | **User-facing HTTP proxy** | `127.0.0.1` |
| `21001…2100N` | Per-artery **dokodemo-door** inbounds (one per line) | `127.0.0.1` |
| `10085` | Existing xray gRPC API (Stats + **add HandlerService**) | `127.0.0.1` |
| `8080` | Combiner WS endpoint (server, Clever Cloud) | `0.0.0.0` |

### 4.4 Data path (Mode B)

```
 App ─► SOCKS5 :10646 / HTTP :10545        (frontend listener; one StreamID per conn)
      ─► framer: OPEN(host:port)+DATA, connection-level Seq, [optional AEAD]
      ─► scheduler: striping (default) or duplication (opt-in)
      ─► raw TCP write to 127.0.0.1:2100x   (dokodemo-door artery inbounds, FollowRedirect off, sniffing off)
      ─► xray outbound_x  ─► CDN edge IP_x / relay_x  ─► (CDN)  ─► Clever Cloud :8080 WS combiner
      ─► combiner: dedup by (StreamID,Seq), reorder, strip frame, dial real dest (youtube:443)
      ◄─ response framed back down active arteries ◄─ client reassembles ◄─ App
```

Each artery inbound is bound to its outbound by a **routing rule** (`inboundTag artery-x → outboundTag artery-x`). The dokodemo **destination** is the *logical combiner address*; reachability is *through* the outbound's CDN edge — keep those two concerns separate.

---

## Part 5 — The adaptive controller (`internal/bonding/control`)

> This is where your "smart, no switching trap, always fastest, self-healing" requirements live. Win-rate alone is **not** enough — it's a duplication-only signal.

### 5.1 Per-path metrics (EWMA, α≈0.125)

`srtt_i, rttvar_i, minRTT_i(10s window)`, `inflight_i` vs `cwnd_i`, BBR-style `deliv_rate_i = bytes_acked/Δt`, `loss_i = retx/sent`, `liveness_i = now − last_ack`. Duplication-only: `win_rate_i = first_arrivals_i / frames`.

### 5.2 Scheduler — **per mode**

* **Duplication (latency/loss-sensitive):** broadcast to all Active paths; combiner takes first by Seq. Win-rate drives Active↔Shadow.
* **Striping (throughput):** **minRTT scheduler with cwnd gating, upgraded to ECF/BLEST** to avoid head-of-line blocking. Per frame: among paths with `inflight_i < cwnd_i`, pick `argmin srtt_i`; **ECF** — don't use a slow path if waiting for the fast one still wins; **BLEST** — suppress a slow-path send that would stall the connection send window. *Plain inverse-RTT weighting is a trap — it ignores cwnd and bloats reordering.* (LEDBAT is congestion control, **not** a scheduler; at most it paces background shadow probes.)

### 5.3 State machine (per path)

States: **ACTIVE → SHADOW → PROBATION → DEAD**. Window `W=5 s`, decisions every `1 s` on EWMA (never single-sample).

* **Demote Active→Shadow:** `srtt_i > 1.5× median(active srtt)` **OR** `loss_i > 5%` **OR** (duplication) `win_rate_i < 10%`, sustained 3 windows.
* **Promote Shadow→Probation:** `srtt` **and** `loss` within `1.2×` of best Active for 3 windows (**two-signal gate**).
* **Probation→Active:** sustains target rate for 2 windows (lets slow-start finish).
* **Dead:** `liveness_i > 3 s`.
* **Anti-switching-trap (this is the "smart threshold" you asked for):** promote bar (`1.2×`) is **strictly tighter** than demote bar (`1.5×`); multi-window confirmation; two-signal gate; **30 s cooldown** before a demoted path may be re-promoted.

### 5.4 Shadow mode — and why "instant promote" isn't free

Keepalives measure **RTT/loss only — never throughput** (you can't measure bandwidth without sending bytes). A freshly promoted path starts at `initcwnd` and must slow-start. Mitigations the roadmap mandates:
1. **Warm-probe tier:** shadows periodically send a small paced burst (a few KB / few s) via `testProxyThroughput()`-style probing → stale-but-real rate estimate + non-zero cwnd.
2. **Duplicate-bridge on promotion:** when an active path degrades, *duplicate* onto the warming shadow first so the user sees no gap, then transition it to striping once its estimator stabilizes.
3. **Make-before-break, always.** Never break-before-make; never inject a cold path straight into Active carrying live frames.

### 5.5 Seamless failover ("retire a line, acquire a new one")

On `DEAD(P)`: (i) immediately **re-queue P's unacked frames onto surviving Active paths** (drain, not reset), preserving Seq; (ii) combiner tolerates the gap within the bounded reorder window; (iii) pull a fresh node from the scanner pool (`ListClientConfigs` sorted by latency, skipping ones already in-pool or recently-failed), **hot-swap** its outbound via HandlerService ([§6/M5](#m5--xray-hot-swap-handlerservice)), register it as **SHADOW**; (iv) warm → PROBATION → ACTIVE. Track a per-node **error budget**; a node that fails K times in a window is **quarantined** (cooldown) before it can re-enter the pool — this is the "retire on errors" loop without thrashing.

### 5.6 Mode selection — **per-flow, one-way upgrade**

You can't know a flow's size up front. Decide at connect time, allow a one-way upgrade:
* Default **Duplication** for interactive/small flows (dest port 53, SSH, RTC/gaming, or initial bytes < 256 KB).
* When a flow crosses **>1 MB or >3 s** continuous transfer → **upgrade to Striping** for the rest of its life (never stripe→dup, to avoid thrash). Per-flow, not per-session.
* Expose a global override toggle (`Auto | Force-Stripe | Force-Duplicate`).

---

## Part 6 — Phased implementation plan

Each milestone is independently shippable, additive, and leaves the existing single-line V2Ray path untouched.

### M0 — Loopback proof-of-correctness (no infra, no real nodes) ★ de-risk first
**Goal:** prove dedup + reorder + striping + failover are byte-exact before any network work.
* `internal/bonding/frame/` — frame encode/decode (§3.1), table-driven tests.
* `internal/bonding/session/` — per-stream reorder buffer, credit window, in-flight tracking.
* `internal/bonding/control/` — scheduler + state machine (§5), pure-function unit tests with synthetic metrics.
* **Test harness:** `SOCKS5 :10646 → framer → N in-process *virtual paths* (goroutines injecting latency/jitter/loss/reorder) → combiner → local echo server`.
* **Acceptance:** byte-exact reassembly under reorder; under one path fully stalled; under a path killed mid-stream (failover re-queue); a **duplication-vs-striping goodput benchmark** that visibly shows the upload-multiplication penalty.
* **Deliverable:** `go test ./internal/bonding/...` green; a `make bonding-bench` target.

> Why first: every later milestone depends on this core being correct, and it needs zero servers. This is the single most important risk-burn-down.

### M1 — Data model & config (additive)
* `internal/models/models.go`: add `BondingEngineConfig` (singleton) and `BondingArtery` (see [§7](#part-7--data-model)). Register in `AutoMigrate`.
* `internal/config/config.go`: add `BondingSocksPort=10646`, `BondingHTTPPort=10545`, `BondingPSKHex`, `BondingCombinerURL`, `BondingMaxArteries`, `BondingFrameSize=4096`, env-driven with defaults.
* **Acceptance:** boots, migrates, config round-trips. No behavior change.

### M2 — Mode A: Selector / Failover engine (high value, no server changes)
**This alone satisfies "fastest line / monitor / fast-switch / retire-and-replace / no switching trap".**
* `internal/bonding/selector/` engine: pull top-N nodes from `pebble.ListClientConfigs(SortBy=latency, limit=N)`; compile a **multi-outbound xray config with the native balancer + observatory** (`compiler.go` already supports `BalancerConfig{leastPing}` + `Observatory` `:701-711`); expose SOCKS `:10646` / HTTP `:10545` as the xray client inbounds.
* Go control loop (the §5.3 state machine, *connection-level granularity*): health-check all lines continuously, drive the balancer's active set, run the §5.5 retire/quarantine/replace loop using `reloadClientCore`-style swaps.
* Lifecycle: `StartEngine/StopEngine/Status` singleton (mirror `soroush/ehcocore` idiom); auto-start from `BondingEngineConfig.IsActive` in `main.go`.
* **Acceptance:** kill the active node → traffic continues on the next-fastest within the demote window, no dropped browser session beyond one connection; a dead node is quarantined and replaced from the pool; no oscillation under jittery latencies.

### M3 — Server combiner on Clever Cloud (WS-over-HTTPS :8080)
* `internal/bonding/server/`: WS listener on `:8080` (mirror `ehcocore` + `nginx.conf`/`Dockerfile` CMD); accept artery connections into a per-bonded-session pool keyed by the handshake session id; run the §3.3 combiner (dedup ring buffer + reorder) and dial real destinations from `OPEN` frames; symmetric downstream framing.
* Single-instance/sticky; PING keepalives.
* Wire into `main.go` server-mode bootstrap (only when `AppMode=="server"`).
* **Acceptance:** a local client (M0 harness pointed at a real `:8080`) streams through the combiner to the internet, byte-exact, survives one artery drop.

### M4 — Mode B client: dokodemo arteries + splitter
* Compiler: `CompileBondingClientConfig(nodes []V2RayClientConfig)` → xray JSON with **one `dokodemo-door` inbound per artery** (`127.0.0.1:2100x`, `FollowRedirect=false`, sniffing **off**, fixed dest = logical combiner) + one outbound per artery + one routing rule per `artery-x` tag. (Reuse the existing API-inbound dokodemo pattern at `compiler.go:316,622`.)
* `internal/bonding/client/`: the §3 frontend — accept on `:10646/:10545`, assign `StreamID`, `OPEN`+`DATA`, run the §5.2 scheduler writing raw TCP to the artery inbounds.
* **Acceptance:** real end-to-end through ≥2 CDN edges to the combiner; striping shows aggregate throughput > best single line on a multi-line test; duplication shows lower tail latency.

### M5 — Xray hot-swap (HandlerService)
**Replaces full-core-restart failover with per-artery swap so other lines never drop.**
* Add `"HandlerService"` to the `api.services` list in **both** `ApiConfig` sites (`compiler.go:294,637`).
* Add a `command.NewHandlerServiceClient` next to the existing `StatsService` client (`interceptor.go`); implement `SwapArtery(tag, newNode)` = `RemoveInbound{tag}`+`RemoveOutbound{tag}` then `AddOutbound{…}`+`AddInbound{…}`. (Per-tag `RemoveHandler` is isolated — other handlers survive.)
* Use it from the §5.5 failover loop; keep full restart only for structural changes. (Persist runtime-added handlers to DB so a supervisor restart can rebuild them.)
* **Acceptance:** swap a dead artery with zero disruption to the other arteries' in-flight streams.

### M6 — Controller integration (full §5)
* Wire metrics from combiner ACKs + PING RTT into the §5.1 estimators; enable ECF/BLEST striping, win-rate duplication, the state machine, warm-probe shadows, duplicate-bridge promotion, per-flow mode selection.
* **Acceptance:** under scripted path degradation (tc/netem on the harness), throughput stays ≥ best single path (no HoL collapse); promotions don't flap; failover is invisible to a running download.

### M7 — API + WebSocket telemetry (`internal/handlers/`, `main.go`)
* Under the protected group (mirror existing `/api/v2ray/...` registration in `main.go:216-304`):
  * `GET/POST /api/v2ray/bonding/config`
  * `POST /api/v2ray/bonding/start` · `POST /api/v2ray/bonding/stop`
  * `GET /api/v2ray/bonding/status`
  * `GET /api/v2ray/bonding/arteries` (pool + states + metrics)
  * `WS /ws/v2ray/bonding/telemetry` (live per-artery stats; mirror `ws.go` + `dashboardStore` stream)
* **Acceptance:** start/stop/status/telemetry all function; 401 enforced.

### M8 — Frontend: the Multipath page ([§9](#part-9--frontend))
* `V2RayMultipathPage.tsx` + `useMultipathStore` (telemetry WS) + the **arteries table** (clone `SubscriptionsCard`) + a **live topology/throughput visualizer** (reuse/extend `SplineChart`) + the **Turbo toggle** (new `Toggle` atom).
* **Acceptance:** the page shows live arteries with state badges (Active/Shadow/Probation/Dead), win-rate/RTT/throughput, and a working Standard↔Turbo toggle that disables manual profile switching while engaged.

### M9 — Hardening
* Reorder/window cap fuzzing; nonce-counter wrap handling; combiner OOM guards; keepalive tuning vs Cloudflare/Sozu idle; reconnect/backoff; metrics export; structured logs via the existing `logger`.

```
M0  loopback correctness ★────────────────────────────────────► foundation
M1  models+config ──► M2 Selector(Mode A, ships value now)
                       │
M0 ─► M3 combiner ─► M4 client arteries ─► M5 hot-swap ─► M6 controller ─► M7 API ─► M8 UI ─► M9 hardening
```

---

## Part 7 — Data model

```go
// internal/models/models.go  (additive; follow existing gorm conventions)

type BondingEngineConfig struct { // singleton, like EhcoServerConfig
    ID            uint   `gorm:"primaryKey" json:"id"`
    IsActive      bool   `json:"is_active"`
    Mode          string `json:"mode"`            // "selector" | "bonding"
    StripingMode  string `json:"striping_mode"`   // "auto" | "stripe" | "duplicate"
    MaxArteries   int    `json:"max_arteries"`    // e.g. 5
    MinArteries   int    `json:"min_arteries"`    // e.g. 2
    CombinerURL   string `json:"combiner_url"`    // wss://app.clever:8080/bond  (bonding mode)
    OriginID      string `json:"origin_id"`       // bonding group origin identity; arteries must match
    PSKHex        string `json:"psk_hex"`         // optional inner AEAD; empty = rely on artery TLS
    FrameSize     int    `json:"frame_size"`      // on-wire, default 4096 (2–4KB)
    // controller thresholds (so they're tunable without a rebuild)
    EvalWindowMs  int     `json:"eval_window_ms"`   // 5000
    DemoteRTTx    float64 `json:"demote_rtt_x"`     // 1.5
    PromoteRTTx   float64 `json:"promote_rtt_x"`    // 1.2
    LossDemotePct float64 `json:"loss_demote_pct"`  // 5
    CooldownSec   int     `json:"cooldown_sec"`     // 30
    ErrorBudget   int     `json:"error_budget"`     // K failures → quarantine
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

type BondingArtery struct { // a line currently in the pool
    ID            uint    `gorm:"primaryKey" json:"id"`
    NodeConfigID  uint    `json:"node_config_id"` // FK → V2RayClientConfig (from pebble pool)
    Tag           string  `json:"tag"`            // "artery-0"
    LocalPort     int     `json:"local_port"`     // 21001…
    State         string  `json:"state"`          // active|shadow|probation|dead|quarantined
    WinRate       float64 `json:"win_rate"`
    SrttMs        float64 `json:"srtt_ms"`
    LossPct       float64 `json:"loss_pct"`
    ThroughputMBps float64 `json:"throughput_mbps"`
    BytesUp       uint64  `json:"bytes_up"`
    BytesDown     uint64  `json:"bytes_down"`
    ErrorCount    int     `json:"error_count"`
    LastSwapAt    time.Time `json:"last_swap_at"`
}
```

(Runtime per-frame state — reorder buffers, in-flight maps, nonce counters — stays in memory in `internal/bonding/session`, never in the DB.)

---

## Part 8 — Risk register

| Risk | Severity | Mitigation (where in roadmap) |
|---|---|---|
| Treating independent egress nodes as bonding paths → stream corruption | **Critical** | Single-egress requirement; origin-tagged groups; Mode A vs B split (§0.1, §4.1) |
| AEAD nonce reuse from global PSK | **Critical** | Default no inner AEAD; else per-session HKDF + directional counters (§3.2) |
| Duplication as default → unusable on Iranian uplink | **High** | Striping default; duplication opt-in FEC only (§0.3, §5.6) |
| QUIC/UDP to Clever origin assumed | **High** | WS-over-HTTPS :8080, inbound-only combiner (§0.4, §4.2) |
| HoL blocking / reorder buffer bloat | **High** | ECF/BLEST scheduler, per-stream credit windows, bounded reorder, 2–4KB frames (§3.3, §5.2) |
| Switching trap / oscillation | **High** | Asymmetric thresholds, multi-window confirm, two-signal gate, 30s cooldown (§5.3) |
| Failover loses in-flight frames | **High** | Drain-not-reset: re-queue unacked onto healthy paths before removal (§5.5) |
| Shadow "instant promote" assumed free | **Medium** | Warm-probe tier + duplicate-bridge + make-before-break (§5.4) |
| Combiner is a SPOF / throughput ceiling | **Medium** | Document; multiple origins (each group pinned to one); relay-fan-in topology B (§4.1) |
| Clever scaling scatters combiner state | **Medium** | Single-instance sticky pin for v1 (§4.2) |
| CDN idle timeouts drop long WS | **Medium** | Mandatory PING keepalives (§4.2) |
| HandlerService not enabled today | **Medium** | M5 enables it in `api.services` (§6/M5) |
| **Dual-use / ToS** | — | Operator-owns-infra framing; this amplifies the existing scanner surface — run only against infrastructure you own or are authorized to use; have an origin-rotation exit plan if blocked |

---

## Part 9 — Frontend

`web/client/src/pages/V2RayMultipathPage.tsx` (new), registered via the `App.tsx:8-26,123-154` idiom + a `Sidebar.tsx` entry + breadcrumb `['V2Ray','Multipath']`.

* **`useMultipathStore`** — clone `dashboardStore.ts:106-426`: `connectStream()` → `WS /ws/v2ray/bonding/telemetry?token=…` → `set({arteries, totals, history})`.
* **Master control card** — `Card` + new **`Toggle` atom** for Standard↔Turbo; mode select (`Selector | Bonding`) and striping override (`Auto | Stripe | Duplicate`); when Turbo is engaged, disable manual profile switching and surface the engine's autonomous state.
* **Topology visualizer** — local node ⇄ Clever origin with one animated artery per line; Active = bright/fast particles scaled by throughput, Shadow = dimmed slow pulse, Probation = amber, Dead = broken red. Reuse/extend `SplineChart` for the aggregate up/down throughput history.
* **Arteries table** — clone `SubscriptionsCard.tsx:1271-1424` (virtualized). Columns: Endpoint (name + country flag + protocol `Badge`), **State** (`Badge` pulse), **Win-Rate %**, **RTT ms** (live), **↑/↓ throughput**, Errors, Actions (pin/evict). Feed from the telemetry store.

All API calls reuse the `Bearer localStorage('cc_client_token')` pattern and the global 401 interceptor.

---

## Part 10 — Definition of done

1. Existing single-line V2Ray flow is **byte-for-byte unchanged** when Turbo is off.
2. **Mode A** ships standalone: continuous health-check, fastest-line selection, sub-second failover, retire/quarantine/replace from the scanner pool, no oscillation.
3. **Mode B** loopback suite is green (M0) **before** any real-node work; real multi-edge striping beats the best single line; duplication lowers tail latency; one-artery failure is invisible to a running download.
4. Combiner runs on Clever Cloud over WS-:8080, single-instance, keepalive-stable.
5. Per-artery hot-swap (HandlerService) swaps a dead line with zero disruption to the others.
6. The Multipath page shows correct live telemetry and a working Turbo toggle.
7. No global-PSK nonce reuse; default path relies on artery TLS 1.3.

---

## Appendix A — Your original blueprint, reconciled

| Your blueprint said | Verdict | Roadmap change |
|---|---|---|
| Feed scanner nodes into an XPlex-style bonding engine | ✅ Sound; scanner already finds clean CDN edges | Require single-origin bonding groups (§4.1) |
| Multiple local Sing-box inbounds → edge outbounds, single core | ✅ | Use **xray** (your default) + **dokodemo-door** fixed-dest inbounds, native multi-inbound compiler (§6/M4) |
| Frame = SessionID+Seq+Len+payload | ❌ insufficient | Add Type + OPEN/CLOSE/RST/PING/WINDOW; 4B StreamID (§3.1) |
| ChaCha20-Poly1305 + global PSK | ❌ unsafe | Default no inner AEAD; else per-session HKDF + directional nonces (§3.2) |
| Duplication = zero loss **and** sums bandwidth | ❌ conflation | Two modes: Redundancy (dup) vs Bonding (stripe); striping default (§0.3, §5.6) |
| Server combiner on :8080 WebSocket/gRPC, reverse pool | ✅ (WS) | Confirmed; **no QUIC/UDP** to Clever (§0.4, §4.2) |
| Win-rate drives Active/Shadow + hysteresis | ⚠️ duplication-only | Add minRTT+ECF/BLEST scheduler for striping; keep win-rate for dup (§5.2) |
| Shadow keepalives; instant promote | ⚠️ | Keepalives = RTT/loss only; warm-probe + duplicate-bridge + make-before-break (§5.4) |
| Dead line → fetch fresh node → hot-reload → inject | ✅ direction right | Drain-not-reset + re-queue unacked + HandlerService hot-swap + quarantine (§5.5, M5) |
| 16 KB frames | ⚠️ | 16 KB only as internal buffer; **2–4 KB on the wire** (§0.4, §3) |
| `ants`, `errgroup`, `deque`, `go-socks5`, chacha20poly1305 packages | ✅ optional | Fine; AEAD only if §3.2 end-to-end mode is enabled |
| SOCKS5 :10646 / HTTP :10545 | ✅ | Kept as the canonical frontend ports (§4.3) |
| Multipath dashboard page + visualizer + telemetry grid + toggle | ✅ | Built in M8 reusing the existing design system (§9) |
```
