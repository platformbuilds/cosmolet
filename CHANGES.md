# Change Notes — `review-overnight` branch

Baseline commit: `9f19922` ("establish review-overnight baseline"). Everything
below is what changed on this branch relative to `main`.

- **Files touched:** 27
- **Committed:** 35 commits (findings 1–34 plus Phase 1–4 docs and the baseline)
- **Uncommitted (working tree):** the full OpenTelemetry integration (Task 15, ROADMAP v0.5.0)

All gates pass: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`
(5 packages, 0 failed, 5 skipped on macOS — loopback/FRR-gated tests that run on
Linux CI), `helm lint charts/cosmolet`.

## Summary

This branch closes a full code review of the cosmolet BGP controller. 35
findings were logged (3 P0, 8 P1, 14 P2, 10 P3) and **all 35 are resolved**:
route withdrawal, leader election, blocking shutdown, FRR robustness (IPv6,
ASN validation, robust output parsing, socket path, persistence-on-change),
chart completeness (RBAC, ConfigMap, probes, image versioning), config
validation, logging, Docker/CI/build fixes, and a real test suite. On top of
that, the ROADMAP v0.5.0 milestone — OpenTelemetry tracing + Prometheus/OTLP
metrics — is implemented end to end (uncommitted, ready to commit).

---

## 1. OpenTelemetry integration (Task 15 / ROADMAP v0.5.0) — uncommitted

Tracing is always on (low-volume per-node daemon); OTLP/HTTP push of traces and
metrics is gated on `otel.enabled && otel.endpoint`. Metrics are always served on
`:8080/metrics` from the OTel SDK's metric pipeline.

| File | Change |
|------|--------|
| `pkg/otel/otel.go` (new) | Bootstraps `TracerProvider` + `MeterProvider`; `resource` with `service.name=cosmolet` and `service.version` (from the `main.Version` ldflag); OTLP/HTTP trace + metric exporters wired only when enabled; tracers `cosmolet.controller` and `cosmolet.frr`; instruments below; `Shutdown(ctx)` flush; `MetricsHandler()` returns a `promhttp` handler backed by the SDK's manual reader. |
| `pkg/otel/otel_test.go` (new) | Bootstrap + instrument tests using manual SDK readers (no network): verifies provider wiring, histogram/gauge/counter/observable-gauge values, and that `MetricsHandler()` serves the pipeline. |
| `pkg/controller/bgp_controller.go` | Nil-safe `SetOTELProvider` + `advertisedRoutesCount`; span per control-loop iteration; FRR spans around `advertiseServiceViaBGP` / `withdrawServiceViaBGP` / `isServiceAdvertisedByFRR`; records all four metrics at the same points. Spans/metrics are no-ops when the provider is nil (tests). |
| `cmd/cosmolet/main.go` | Bootstraps the OTel provider from `cfg.OTEL`; replaces the static `cosmolet_info` stub with the real `otelProvider.MetricsHandler()` on `:8080/metrics`; calls `otelProvider.Shutdown` on SIGTERM. |
| `pkg/config/config.go` | New `otel` block (`enabled`, `endpoint`, `sample_ratio`) + `Validate()` (sample ratio in `0..1`; endpoint required when enabled). |
| `charts/cosmolet/values.yaml` | `config.otel` knob, default `enabled: false`. |
| `charts/cosmolet/templates/configmap.yaml` | Renders the `otel` block into `config.yaml`. |
| `README.md` | New "Monitoring" section: metrics table, OTLP push, `otel` config example. |
| `ROADMAP.md` | v0.5.0 row marked done. |
| `go.mod` / `go.sum` | Added `go.opentelemetry.io/otel` v1.47.0 (+ `/sdk`, `/sdk/metric`, `/exporters/otlp/otlptrace/otlptracehttp`, `/exporters/otlp/otlpmetric/otlpmetrichttp`) and `github.com/prometheus/client_golang` v1.25.0. |

**Metrics emitted**

| Metric | Type | Description |
|--------|------|-------------|
| `cosmolet_loop_duration_seconds` | Histogram | Duration of one control-loop iteration |
| `cosmolet_services_discovered` | Gauge | Services discovered in the last loop |
| `cosmolet_bgp_operations_total` | Counter | BGP ops, labelled `op` (advertise/withdraw/check) and `outcome` (success/failure) |
| `cosmolet_advertised_routes` | Gauge (observable) | Routes currently advertised |

**Config example**

```yaml
otel:
  enabled: true
  endpoint: "http://otel-collector:4318"   # host:port or http(s) URL
  sample_ratio: 1.0   # 0..1; values outside (0,1) always sample
```

---

## 2. Committed changes by phase

### Phase 1 — Design & review documentation
- `f77a114` Phase 1 "design as documented" section.
- `21c393c` Phase 1 design-vs-implementation table.
- `REVIEW.md` (created): the full findings log, design tables, executive summary, and test-coverage table.

### Phase 2 — Findings 1–34 (P0 → P3)

**Correctness (controller core loop)**
- `bb2cdce` **Finding 1 (P0):** route withdrawal for deleted/unhealthy services — previously routes were never withdrawn (traffic blackholing). Added `withdrawStaleRoutes` / `withdrawAllRoutes` + advertised-IP tracking.
- `8304dc9` **Finding 7 (P1):** `CommandRunner` interface; no longer marks a route advertised when `vtysh` fails (retry next loop).
- `979d1c6` **Findings 2, 3, 4, 6, 11, 12 (P1/P2):**
  - Finding 2: Lease-based **leader election** (`leaderelection.RunOrDie`); only the leader mutates BGP state.
  - Finding 3: **blocking shutdown** — `Start` returns only after `withdrawAllRoutes` completes.
  - Finding 4: fail hard when `ip addr add` fails (no more blackholed routes).
  - Finding 6: 10 s timeout on every `vtysh`/`ip` command (a hung FRR can't wedge the loop).
  - Finding 11: wire `frr.socket_path` to `vtysh -s`.
  - Finding 12: `write memory` (persist) only on route-set change, not every loop.
- `a1e5e85` **Finding 5 (P2):** transient-vs-deleted health handling (`healthResult` / `processOutcome` / `withdrawStaleRoutes(confirmed, keep)`) so a single API blip can't flap a route.

**FRR integration**
- `ce1df36` **Findings 8, 9, 10 (P1/P2):** ASN validation (`1..4294967295`), **IPv6** support (`/128`, `ipv6 unicast`, `show bgp ipv6 unicast`), robust BGP output parse (`*>` local-origin marker).
- `a1e5e85` removed the dead `frr.config_path` knob.

**Helm chart**
- `0767959` **Finding 13 (P1):** ConfigMap template rendering the snake_case config.
- `50ca131` **Finding 14 (P0):** RBAC templates (ServiceAccount, ClusterRole, ClusterRoleBinding) + `coordination.k8s.io/leases` for leader election.
- `70db0fe` **Findings 15/16/18:** single source of truth for image version; fixed registry/repo mismatch; cleaned placeholders.
- `f0406af` **Finding:** removed dead `replicaCount` value.
- `a1e5e85` **Finding 19 (P3):** in-pod `livenessProbe` / `readinessProbe` (the Dockerfile `HEALTHCHECK` is ignored by kubelet) + `probes` values knob.
- `daemonset.yaml`: Downward-API `POD_NAME` / `POD_NAMESPACE` env (leader-election identity + Lease namespace); image ref now includes the registry.
- `Chart.yaml`: real maintainer/home URLs (was placeholder).

**Config & ops**
- `f8cd3ee` **Findings 22, 23, 24:** reject empty namespaces, cap `loop_interval_seconds` (3600), fail closed on a missing config file.
- `b74f9cd` **Finding 21 (P1):** apply the documented `-log-level` flag and `logging.level` (was a dead knob).
- `c2fb03d` **Finding 25 (P3):** `ioutil.ReadFile` → `os.ReadFile`.
- `a1e5e85` **Finding 34 (P3):** JSON log formatter (`-log-format json`) — new `SetLevel`/`SetFormat`/json output in `pkg/logging/logging.go`.

**Docker / CI / build**
- `38ba404` **Finding 20 (P0):** Dockerfile builder `golang:1.21-alpine` → `golang:1.27-alpine` to satisfy `go 1.27.3`.
- `287e2e4` **Finding 30:** `go mod tidy` → `go mod verify` at build time (reproducible builds).
- `a67a7ac` **Finding 29 (P3):** `.dockerignore` so `COPY . .` excludes non-build inputs.
- `0dc017d` **Finding 28 (P1):** GitHub Actions CI (build + vet + `test -race`).
- `96e577a` **Finding 26 (P2):** stop gitignoring `go.mod`/`go.sum` (now tracked).
- `09781e4` **Finding 27 (P2):** removed the stale 51 MB darwin-arm64 binary from git.
- `1490015` **Finding 33 (P3):** `scripts/build.sh` now sets `GitCommit` and `BuildDate` ldflags.
- `Makefile` **Finding 32 (P2):** version fallback `unreleased-<commit>` when no tag exists (*creating the first tag is a human decision*).
- `a1e5e85` **Finding 31 (P2):** `EXPOSE 8080 9090` → `EXPOSE 8080` (dropped the dead port).

### Phase 3 — Tests
- `bc4e777` `pkg/config` tests (`LoadConfig`, accessors) — 73.7% → 95.0%.
- `3f9ace1` `pkg/controller` control-loop tests (fake clientset) — 23.8% → 56.5%.
- `f9f6e70` `pkg/health` checker tests (liveness, readiness, concurrency) — 0% → 100%.
- `2d6b071` Phase 3 coverage table + **Finding 35** (`lo`/`lo0` loopback portability, fixed in `bgp_controller.go`).

### Phase 4 — Wrap-up docs
- `67e715d` Executive Summary + final running summary.
- `e99ef50` / `47bfa8a` / `7230d7c` / `3f66ef3` / `f50e682` / `90d64b5` / `7c92ef9` / `277772c` — per-task commit hashes and progress-log entries in `PROGRESS.md`.

---

## 3. Files changed (relative to baseline `9f19922`)

```
 .dockerignore                              |  18 +
 .github/workflows/ci.yml                   |  28 +
 .gitignore                                 |   3 -
 Dockerfile                                 |   8 +-
 Makefile                                   |  11 +-
 PROGRESS.md                                | 311 +
 README.md                                  |  32 +
 REVIEW.md                                  | 319 +
 ROADMAP.md                                 |   2 +-
 bin/cosmolet-darwin-arm64                  | Bin 51425586 -> 0 bytes
 charts/cosmolet/Chart.yaml                 |  10 +-
 charts/cosmolet/templates/configmap.yaml   |  28 +
 charts/cosmolet/templates/daemonset.yaml   |  25 +-
 charts/cosmolet/templates/rbac.yaml        |  62 +
 charts/cosmolet/values.yaml                |  50 +-
 cmd/cosmolet/main.go                       | 148 +-
 go.mod                                     |  64 +-
 go.sum                                     | 149 +-
 pkg/config/config.go                       |  73 +-
 pkg/config/config_test.go                  | 396 +
 pkg/controller/bgp_controller.go           | 624 +-
 pkg/controller/bgp_controller_loop_test.go | 611 +
 pkg/controller/bgp_controller_test.go      | 429 +
 pkg/health/checker_test.go                 | 379 +
 pkg/logging/logging.go                     | 159 +
 pkg/logging/logging_test.go                | 148 +
 scripts/build.sh                           |   4 +-
 pkg/otel/otel.go                           | 307 +   (uncommitted)
 pkg/otel/otel_test.go                      |  ~150 + (uncommitted)
```

## 4. Test coverage (final)

| Package | Coverage |
|---------|----------|
| `pkg/config` | 95.0% |
| `pkg/health` | 100% |
| `pkg/logging` | 93.8% |
| `pkg/controller` | 56.5% (5 loopback/FRR-gated tests skip on macOS; run on Linux CI) |
| `pkg/otel` | new (bootstrap + instrument tests, no network) |
| `cmd/cosmolet` | 0% (thin `main`) |

## 5. Remaining human decisions (not code findings)

- Create the first git release tag (e.g. `v0.1.0`) so `make docker-build` emits a
  semantic image tag (Finding 32 residual).
- Privileged / container-hardening posture and "who runs the FRR daemon"
  (Design-vs-Implementation table items).
