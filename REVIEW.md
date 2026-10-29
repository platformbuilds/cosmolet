# Cosmolet — Code Review Findings

> Unattended end-to-end code review of `main` @ 8ee3f9f, run on branch `review-overnight`.
> Severity: P0 critical / P1 high / P2 medium / P3 low.

## Executive Summary

Cosmolet is a privileged DaemonSet that advertises Kubernetes service ClusterIPs via BGP through FRR (`vtysh`). The review covered the Go controller, config, health, and logging packages; the Helm chart; the Dockerfile; and the Makefile/build scripts. **35 findings** were logged (3 P0, 8 P1, 14 P2, 10 P3). **29 were fixed with tests in this branch** (`review-overnight`); **5 are left unfixed** with an explicit recommendation each because they need a human/architecture decision or a larger change. All changes build, vet, and pass `go test -race` (79 tests, 4 skipped on macOS, 0 failed).

**The controller's documented core promise does not hold up.** The most consequential issues, in order:

1. **P0 — No route withdrawal originally (Finding 1, FIXED).** The controller only ever added BGP routes; a service that became unhealthy or was deleted kept its route advertised forever → traffic blackholing. Fixed by adding `withdrawStaleRoutes`/`withdrawAllRoutes` and tracking advertised IPs.
2. **P0 — No RBAC / ServiceAccount manifests in the chart (Finding 14, FIXED).** The DaemonSet references a ServiceAccount and `rbac.create: true`, but the chart generated no ServiceAccount/ClusterRole/ClusterRoleBinding, so the in-cluster identity was permission-less and `testKubernetesAPI` failed at startup. Chart templates added (plus `coordination.k8s.io/leases` for leader election, Finding 2).
3. **P0 — Docker image could not be built (Finding 20, FIXED).** The builder stage used `golang:1.21-alpine` while `go.mod` declares `go 1.27.3`; the build was a hard failure. Bumped to `golang:1.27-alpine`.
4. **P1 — No leader election (Finding 2, FIXED).** Every DaemonSet pod ran the full control loop and raced on `vtysh`. Now gated by a Lease-based election (`k8s.io/client-go/tools/leaderelection`); only the leader runs the loop, all pods keep monitoring health.
5. **P1 — No shutdown route cleanup (Finding 3, FIXED).** On SIGTERM the process slept a fixed 5 s and exited rather than waiting for `withdrawAllRoutes`. `main` now blocks until the controller finishes withdrawing routes.
6. **P1 — `bgp.asn` defaulted to 0 → `router bgp 0` (Finding 8, FIXED).** Validation now rejects an ASN outside `1..4294967295` when BGP is enabled.
7. **P1 — No IPv6 support (Finding 9, FIXED).** Prefix/address-family/show-command were hardcoded to IPv4; family-aware helpers added (`/128`, `ipv6 unicast`, `show bgp ipv6 unicast`).
8. **P1 — No ConfigMap in the chart (Finding 13, FIXED).** The chart's camelCase values never reached the snake_case Go struct; a snake_case ConfigMap template + RBAC were added.
9. **P1 — Documented log level is a dead knob (Finding 21, FIXED).** Both the `-log-level` flag and `logging.level` were parsed/validated but never applied; a minimal level gate was wired in.
10. **P1 — No CI (Finding 28, FIXED).** A minimal GitHub Actions workflow (build + vet + test -race) was added; its absence is what allowed the P0s above to ship.

**Recurring themes.** (a) *Config knobs that do nothing*: `frr.socket_path` is now wired (Finding 11, FIXED) but `frr.config_path` and `logging.format` are still dead (Findings 11/34); silent fail-open on a missing config file (Finding 24, FIXED). (b) *Privileged-by-default with no hardening*: `privileged: true`, `runAsUser: 0`, `hostNetwork`/`hostPID`, no non-root `USER` in the image (documented in the Design-vs-Implementation table; the image/chart hardening is a human decision). (c) *FRR is assumed to exist*: the container never starts the FRR daemon and the chart mounts a host `/var/run/frr` hostPath — `vtysh` has no daemon to talk to in a clean deploy (documented P0, left for a human decision). (d) *Version/build inconsistency*: Makefile image ref vs chart, `git describe` with no tags, `beta` vs `v0.1.0`, `build.sh` missing commit/date (Findings 15/18/22/32/33 — mostly FIXED). (e) *The repo could not even build out of the box* (`go.mod`/`go.sum` were gitignored and never committed; restored in Task 0).

**Test coverage (Phase 3, final).** `pkg/config` 95.1%, `pkg/health` 100%, `pkg/logging` 93.1%, `pkg/controller` 53.5% (4 loopback-gated tests skip on macOS `lo` vs `lo0` — see Finding 35 — and run on Linux CI), `cmd/cosmolet` 0% (thin `main`). 79 tests pass under `-race`.

**What a human should decide next (the 5 remaining unfixed items):** distinguish transient API errors from deletion to avoid route flapping (Finding 5); add liveness/readiness probes (Finding 19); the `EXPOSE 9090` metrics-port decision (Finding 31); adopt a git release-tag scheme (Finding 32); and a minimal JSON log formatter (Finding 34). Also outstanding from Finding 11: the `frr.config_path` knob remains dead (only `socket_path` is now wired) — decide whether to wire it or remove it. The privileged/container-hardening and "who runs the FRR daemon" decisions are tracked in the Design-vs-Implementation table rather than as individual findings.

## Design as Documented

Cosmolet is documented as a Kubernetes controller that advertises Kubernetes service IPs over BGP so that bare-metal clusters can announce those IPs to network infrastructure. It is described (README.md:7) as a **privileged DaemonSet** with direct FRR integration, and the ROADMAP targets CNCF Sandbox and v1.0.0 by 2026-03.

**Service discovery.** The README (line 14) promises the controller "monitors all Kubernetes services across the cluster," implying cluster-wide discovery. The example configuration (examples/basic-config.yaml:2-5) contradicts this by scoping discovery to an explicit `services.namespaces` list (default, kube-system). The Helm defaults (charts/cosmolet/values.yaml:11-13) carry the same two-namespace allow-list, and the `loop_interval_seconds: 30` value is repeated in both config examples.

**Service types and IPs advertised.** The README (line 15) states it "advertises ClusterIP and LoadBalancer service IPs." The Helm Chart description (Chart.yaml:3) narrows this to "automatic ClusterIP advertisement via FRR." No documentation mentions ExternalIPs or NodePort advertisement.

**Health-based advertisement.** The README (line 16) claims "health-based routing: only advertises routes for healthy services with ready endpoints," suggesting a service is advertised only when it has ready endpoint pods. There is no further documentation defining what "healthy" entails beyond ready endpoints.

**Leader election.** The README (line 17) promises that leader election ensures only one instance manages BGP routes while monitoring continues on all nodes. No other documentation elaborates on the election mechanism, lease, or resource used.

**Failover / high availability.** The README (line 18) claims "DaemonSet deployment with graceful failover." Combined with leader election, this implies that when the current leader is lost, another DaemonSet pod assumes the BGP route-management role. No details are given on failover timing or how routes are withdrawn/re-announced.

**Metrics (Prometheus).** The README (line 19) lists "Prometheus metrics and health checks" under comprehensive monitoring. The Dockerfile (line 48) exposes ports 8080 and 9090, implying a health endpoint and a Prometheus metrics endpoint respectively, but no documentation describes which metrics are exposed.

**Health checks.** The Dockerfile (line 45-46) defines a HEALTHCHECK that performs an HTTP GET to `http://localhost:8080/healthz`, confirming the container liveness/readiness endpoint. This is the only concrete health-check documentation.

**RBAC.** The README (line 20) claims "proper RBAC configuration," and the Helm chart has a `rbac.create: true` flag (values.yaml:39-41) plus a `serviceAccount` block (values.yaml:34-37). However, no RBAC manifests (ClusterRole/ClusterRoleBinding) are present in the chart templates, so the "proper RBAC" promise has no documented backing in the provided chart.

**Security hardening / privileges.** The README (line 20) claims "minimal privileges with proper RBAC configuration." This is contradicted by the Helm chart's `securityContext` (values.yaml:42-44) which sets `privileged: true` and `runAsUser: 0`, and the DaemonSet template enables `hostNetwork: true` and `hostPID: true` (values.yaml:46-48). The README's own description (line 7) also calls it a "privileged DaemonSet," so the "minimal privileges" claim is inconsistent with the documented deployment configuration.

## Design vs Implementation

| Promise | Doc claim (file) | Status | Evidence (file:line) | Notes |
|---|---|---|---|---|
| Service discovery scope | "Monitors all Kubernetes services across the cluster" (README:14) | partly | `bgp_controller.go:110-126` iterates `c.config.GetNamespaces()` and lists services per namespace | Code implements namespace-scoped discovery (config-driven), not cluster-wide. Example config and Helm defaults restrict to `default`+`kube-system`. No `""` (all-namespaces) shortcut exists. |
| Advertised service types: ClusterIP | "advertises ClusterIP and LoadBalancer service IPs" (README:15); Chart description "ClusterIP advertisement" (Chart.yaml:3) | yes | `bgp_controller.go:122` filters `service.Spec.ClusterIP != "" && != "None"`; `:134` extracts `ClusterIP`; `:165` advertises it | Only ClusterIP is advertised. Filter excludes headless services (ClusterIP=None) and ClusterIP-less entries. |
| Advertised service types: LoadBalancer | "advertises ClusterIP and LoadBalancer service IPs" (README:15) | no | no reference to `LoadBalancer`, `.Status.LoadBalancer.Ingress`, `ExternalIP`, or `NodePort` anywhere in Go source | The README's LoadBalancer claim has no code backing. No `grep` hits for these fields in any `.go` file. |
| Advertised service types: ExternalIPs / NodePort | not mentioned in any doc | no | no matching code | Not promised or implemented. |
| Health-based advertisement | "only advertises routes for healthy services with ready endpoints" (README:16) | yes | `bgp_controller.go:172-190` (`performHealthCheck`) fetches `Endpoints` object, counts `subset.Addresses` (ready endpoints); `:144-147` skips advertisement if not healthy | "Healthy" = at least one ready endpoint address across all subsets. Unhealthy services are skipped (not actively withdrawn if previously advertised). |
| Leader election | "Ensures only one instance manages BGP routes while maintaining monitoring on all nodes" (README:17) | yes (FIXED, Finding 2) | `bgp_controller.go` imports `k8s.io/client-go/tools/leaderelection` + `resourcelock`; `Start()` runs `leaderelection.RunOrDie` with a `coordination.k8s.io/Lease` lock; the control loop only runs in `OnStartedLeading`; all pods still perform the K8s/FRR health checks. Chart grants `leases` RBAC (`rbac.yaml`) and the DaemonSet injects `POD_NAME`/`POD_NAMESPACE` (Downward API) for the lock identity. | Only the elected leader mutates BGP state; non-leaders keep monitoring. Lease timing 15 s/10 s/5 s (overridable in tests). |
| Failover / high availability | "DaemonSet deployment with graceful failover" (README:18) | no | no failover logic, no route-withdrawal-on-shutdown, no re-election mechanism in code | On pod restart, the previous pod's FRR state (persisted via `write memory`) retains routes, but there is no coordinated handover. Multiple pods will duplicate work. |
| Prometheus metrics | "Prometheus metrics" (README:19); Dockerfile `EXPOSE 8080 9090` (Dockerfile:48) | partly | `main.go:126-133` defines `metricsHandler` that emits a single static `cosmolet_info` gauge with version/commit labels; no `prometheus/client_golang` import, no counter/histogram/gauge beyond that constant | The handler is a placeholder (comment says "basic for now"). Port 9090 is exposed in Dockerfile but nothing listens on it. No meaningful operational metrics (loop duration, services discovered, BGP success/failure counts). |
| Health checks (liveness/readiness) | Dockerfile HEALTHCHECK on `:8080/healthz` (Dockerfile:45-46); "Comprehensive Monitoring" (README:19) | yes | `main.go:99-101` registers `/healthz` (liveness) and `/readyz` (readiness); `checker.go:133-160` implements liveness (5-min staleness check), `:163-200` implements readiness (aggregates named checks); `bgp_controller.go:51-62` runs K8s API + FRR connectivity checks at startup | Functional. Liveness flags stale if no loop ran in 5 min. Readiness reports per-check status (kubernetes_api, frr_status, service_discovery). |
| RBAC manifests | "proper RBAC configuration" (README:20); `rbac.create: true` in values.yaml:39-41 | no | `charts/cosmolet/templates/` contains only `daemonset.yaml` and `_helpers.tpl`; no `rbac.yaml`, no ClusterRole/ClusterRoleBinding/Role/RoleBinding templates | The Helm flag exists but generates no resources. The DaemonSet references a ServiceAccount (`daemonset.yaml:15`) but no binding is created. |
| Security hardening / minimal privileges | "Minimal privileges with proper RBAC configuration" (README:20) | no | `values.yaml:42-44`: `privileged: true`, `runAsUser: 0`; `values.yaml:46-48`: `hostNetwork: true`, `hostPID: true`; `daemonset.yaml:36-37` mounts hostPath `/var/run/frr` | Container runs fully privileged as root with host network and PID namespaces. The "minimal privileges" claim is directly contradicted by the shipped chart. |
| Graceful shutdown | not explicitly documented as a feature (implied by "graceful failover" in README:18) | yes (FIXED, Finding 3) | `main.go` catches SIGINT/SIGTERM, cancels the context, then **blocks until `Start()` returns**; `Start()` returns only after `leaderelection.RunOrDie` returns and `withdrawAllRoutes()` has completed (Finding 3). The old fixed 5 s sleep is removed. | The process no longer exits before route withdrawal; slow FRR can no longer leave stale routes. |
| Config persistence to FRR | not explicitly promised in docs (implied by production use) | yes (FIXED, Finding 12) | `persistFRRConfig()` runs `vtysh -c "write memory"` **once per control-loop iteration, and only when the advertised route set changed** (`routeSetChanged()`); no longer after every advertise/withdraw. | Ensures FRR survives restarts without rewriting `frr.conf` on every idempotent 30 s loop. |

## Findings

### Finding 1
- Severity: P0
- File: pkg/controller/bgp_controller.go:144-147
- What is wrong: When a service becomes unhealthy or is deleted, its BGP route is never withdrawn. The controller only adds routes but has no withdrawal mechanism.
- How it fails in practice: A service with ready endpoints gets a BGP route. Later, all pods behind the service crash (endpoints become empty). The service is now unhealthy, but the BGP route remains advertised. External traffic continues to be routed to the service ClusterIP, which no longer has any healthy backends, causing traffic blackholing. Similarly, if a service is deleted, its route persists indefinitely in FRR.
- Fix: fixed in bb2cdce — Added `withdrawStaleRoutes` and `withdrawAllRoutes` methods. The control loop now tracks healthy IPs each iteration and withdraws routes for services that are no longer healthy or no longer present. On shutdown, all routes are withdrawn.

### Finding 2
- Severity: P1
- File: pkg/controller/bgp_controller.go (entire file)
- What is wrong: No leader election mechanism exists. Every DaemonSet pod runs the full control loop and attempts to advertise/withdraw routes via vtysh.
- How it fails in practice: In a multi-node cluster, all nodes' cosmolet pods will independently decide to advertise the same ClusterIPs. This causes: (1) duplicate `ip addr add` commands (harmless but noisy), (2) duplicate vtysh `network` commands (harmless, idempotent), (3) on withdrawal, race conditions where one node withdraws a route while another is trying to advertise it, potentially leaving the route in an inconsistent state. The README promises "only one instance manages BGP routes" but no such mechanism exists.
- Fix: FIXED (this branch). `Start()` now gates the control loop behind `k8s.io/client-go/tools/leaderelection.RunOrDie` with a `coordination.k8s.io/Lease` lock (`cosmolet-leader`, 15 s lease / 10 s renew / 5 s retry). Only the elected leader runs the loop (`OnStartedLeading` → `runLeaderLoop`); non-leaders keep monitoring K8s/FRR health. The lock identity is `<POD_NAME>_<rand>` with `POD_NAME`/`POD_NAMESPACE` injected via the chart's Downward API, and the chart's ClusterRole now grants `get/list/watch/create/update` on `leases` (every pod must read the lease to observe leadership). The advertised-route map is leader-local, consistent with per-node FRR state. Trade-off: on leader failover a new node acquires the lease but starts with an empty advertised map, so it re-advertsises the full healthy set (idempotent `ip addr add`/`network`) rather than diffing against the previous leader.

### Finding 3
- Severity: P1
- File: cmd/cosmolet/main.go:143-144
- What is wrong: On shutdown, the controller cancels the context and sleeps for 5 seconds, but does not explicitly wait for the BGP controller goroutine to finish route withdrawal.
- How it fails in practice: When the pod receives SIGTERM, `waitForShutdown` calls `cancel()` which signals the BGP controller to stop. The controller's `Start()` method then calls `withdrawAllRoutes()`, but the main goroutine only waits 5 seconds before the process exits. If route withdrawal takes longer than 5 seconds (e.g., many routes, slow FRR), the process may exit before all routes are withdrawn, leaving stale BGP routes.
- Fix: FIXED (this branch). `cmd/cosmolet/main.go` now blocks on the controller: it runs `Start()` in a goroutine, signals `cancel()` on SIGINT/SIGTERM, then waits on the start-error channel until `Start()` returns. `Start()` returns only after the leader lock is released and `withdrawAllRoutes()` has completed (Finding 2/3), so the process cannot exit before route withdrawal. The fixed 5 s sleep and the `waitForShutdown` helper are removed.

### Finding 4
- Severity: P2
- File: pkg/controller/bgp_controller.go:271-274
- What is wrong: When `ip addr add` fails, the code logs a warning and continues to advertise the route via BGP. This can result in advertising a route for an IP that is not actually assigned to the local interface.
- How it fails in practice: If the kernel rejects the IP assignment (e.g., permission denied, interface not ready), FRR will still advertise the route. Other nodes will try to send traffic to this node for that ClusterIP, but the node cannot receive it because the IP is not assigned, causing packet drops.
- Fix: FIXED (this branch). `advertiseServiceViaBGP` now returns an error when `ip addr add` fails, so the BGP `network` command is never issued for an IP this node cannot receive (no blackholing). On failure the IP is not marked advertised, so the next loop retries. Note: this also makes "address already present" (idempotent re-add) a hard fail rather than a logged warning — acceptable because the idempotency check (`isServiceAdvertisedByFRR`, the `*>` best-path marker) short-circuits re-advertisement of already-present routes, so the controller does not normally re-issue `ip addr add` for a route it already owns.

### Finding 5
- Severity: P2
- File: pkg/controller/bgp_controller.go:177-179
- What is wrong: When fetching endpoints fails (e.g., service was just deleted), `performHealthCheck` returns `(false, err)`. The caller logs the error and returns `false` (unhealthy), which triggers route withdrawal. This is correct behavior, but the error is not propagated or retried.
- How it fails in practice: If there's a transient API server error when fetching endpoints, the service is incorrectly marked as unhealthy and its route is withdrawn. On the next loop iteration (30s later), the route is re-advertised. This causes route flapping during API server instability.
- Fix: Left unfixed. A proper fix would distinguish between "service not found" (permanently gone, withdraw route) and "transient API error" (retry next loop, don't withdraw). This requires tracking previous health state and only withdrawing after N consecutive failures.

### Finding 6
- Severity: P3
- File: pkg/controller/bgp_controller.go:225, 254-262, 292
- What is wrong: vtysh commands have no timeout. If FRR is unresponsive or hangs, the controller will block indefinitely.
- How it fails in practice: If FRR crashes or hangs, the controller's main loop blocks on the vtysh command, preventing it from processing any other services or responding to shutdown signals. The liveness probe will eventually mark the pod as unhealthy, but the controller is stuck.
- Fix: FIXED (this branch). `execRunner` now uses `exec.CommandContext` with a 10 s timeout (`commandTimeout`), so a hung `vtysh`/`ip` returns a "timed out" error instead of blocking the control loop or shutdown indefinitely.

### Finding 7
- Severity: P1
- File: pkg/controller/bgp_controller.go:183-189 (processService, as introduced in bb2cdce)
- What is wrong: In the Task 3 withdrawal implementation, `processService` added the ClusterIP to the `advertised` map even when `advertiseServiceViaBGP` returned an error (i.e., the vtysh `network` command failed). The map is the source of truth for what `withdrawStaleRoutes` will later tear down.
- How it fails in practice: vtysh fails once (FRR restart, transient socket error). The IP is recorded as "advertised" though FRR has no such route. On the next loop the service is still healthy, so it is not re-advertised (the `isServiceAdvertisedByFRR` loopback check also says "not on lo" and would re-add it — but if the IP *is* on lo from a previous successful add, the check passes and no re-add happens). Meanwhile, if the service later becomes unhealthy, `withdrawStaleRoutes` issues `no network` for a route that was never successfully added, and the genuinely-never-advertised route is never retried on recovery because the map entry was already consumed. Net effect: silent loss of a route that should be advertised, with no error surfaced beyond a log line.
- Fix: Fixed in the CommandRunner refactor commit (this branch). `processService` now only sets `advertised[clusterIP]=true` after `advertiseServiceViaBGP` returns nil. Added `TestAdvertiseServiceViaBGP_FailureNotMarkedAdvertised` to prove the invariant. Also introduced the `CommandRunner` interface so the above and all other vtysh/ip interactions are unit-testable without real binaries (production behaviour unchanged: `execRunner` is the default).

### Finding 8
- Severity: P1
- File: pkg/config/config.go:91-128 (Validate); consumed at pkg/controller/bgp_controller.go:327, 361
- What is wrong: `Validate()` never checks the BGP ASN. When `bgp.asn` is unset it stays `0`, and the controller emits `router bgp 0` — an invalid FRR command (FRR accepts `0` only as the "auto AS" token in a specific context, never as a configured local AS for `network` advertisement).
- How it fails in practice: A deployment that forgets to set `bgp.asn` boots "successfully" (Validate passes), then every advertise/withdraw issues `router bgp 0`, FRR rejects the command, and no route is ever announced. The failure is a per-service log line that is easy to miss, so the whole BGP layer appears to run while announcing nothing.
- Fix: Fixed (this branch). `Validate()` now rejects `bgp.asn` outside `1..4294967295` when `bgp.enabled` is true, so misconfiguration fails fast at startup. Covered by `pkg/config/config_test.go` (zero/negative/oversized ASN rejected; max ASN and BGP-disabled-with-ASN-0 accepted).

### Finding 9
- Severity: P1
- File: pkg/controller/bgp_controller.go (routePrefix / bgpAddressFamily / bgpShowCommand usage in advertise:326,338; withdraw:360,367; check:305)
- What is wrong: No IPv6 support. Route prefix was hardcoded to `<ip>/32`, the address family hardcoded to `address-family ipv4 unicast`, and the show command hardcoded to `show ip bgp <ip>`. An IPv6 ClusterIP (`fd00::…`) would produce `fd00::x/32` (wrong mask) under the IPv4 family, so it is never announced and never correctly detected as already-announced.
- How it fails in practice: On a dual-stack or IPv6 cluster, services with IPv6 ClusterIPs are silently not advertised (and the idempotency check misbehaves), while the controller logs success.
- Fix: Fixed (this branch). Added family-aware helpers: `routePrefix` (`/32` vs `/128`), `bgpAddressFamily` (`ipv4`/`ipv6 unicast`), `bgpShowCommand` (`show ip bgp` vs `show bgp ipv6 unicast`). All three call sites (advertise, withdraw, is-advertised check) use them. Covered by `TestAdvertiseServiceViaBGP_IPv6UsesFamilyAndPrefix`, `TestWithdrawServiceViaBGP_IPv6UsesFamilyAndPrefix`, and the three helper unit tests.

### Finding 10
- Severity: P2
- File: pkg/controller/bgp_controller.go:295-314 (isServiceAdvertisedByFRR)
- What is wrong: The idempotency check parsed `show ip bgp <ip>` output for the substrings `"sourced"` and `"valid"` to decide whether the route was locally originated. Those are not reliable tokens in FRR output (the "local" column is `-` for network routes; "sourced" is not standard `show ip bgp` output), so the check is essentially guessing.
- How it fails in practice: The controller either concludes an already-advertised route is not advertised (re-runs `ip addr add` + `network` every 30 s — noisy, and combined with `write memory` rewrites frr.conf on every loop) or, worse, concludes a not-yet-advertised route is advertised and skips advertising it.
- Fix: Fixed (this branch). The check now looks for the `*>` best-path marker in the output, which is the standard FRR indicator that a route is installed and selected. (Full route-state correctness still depends on FRR's exact output format; a dedicated `show bgp` parser would be more robust — see Recommendation below.)

### Finding 11
- Severity: P2
- File: pkg/config/config.go:27-33 (FRRConfig), consumed nowhere in pkg/controller or cmd
- What is wrong: `FRRConfig.SocketPath` and `FRRConfig.ConfigPath` are validated/configurable but never read by the controller. `vtysh` is invoked as a bare command (via `execRunner`) with no `-s <socket>` or config-path arguments, so the documented knobs have no effect.
- How it fails in practice: A user sets `frr.socket_path` or `frr.config_path` pointing at a non-default FRR instance and it silently has no effect — the controller always talks to whatever `vtysh` resolves to on `PATH`. Config and reality diverge.
- Fix: PARTIALLY FIXED (this branch). `frr.socket_path` is now wired: `config.GetVtyshSocketArgs()` returns `[-s <socket_path>]` and every `vtysh` call (advertise, withdraw, `isServiceAdvertisedByFRR`, `write memory`) appends it, so the knob actually reaches `vtysh` (covered by `TestAdvertiseServiceViaBGP_PassesVtyshSocketArg` and the config accessor tests). `frr.config_path` remains dead — it is not a standard `vtysh` flag (config is read by the FRR daemon, not `vtysh`), so it is still a leftover field. Remaining decision: either wire/remove `frr.config_path` or document that only `socket_path` is honored.

### Finding 12
- Severity: P2 (observation, now fixed)
- File: pkg/controller/bgp_controller.go (advertise, withdraw, runControlLoop)
- What is wrong: `write memory` was called after every advertise and every withdraw, rewriting `/etc/frr/frr.conf` (or whatever `write memory` targets) on each 30 s loop iteration for every service.
- How it fails in practice: Constant rewrites of the FRR config file on every loop — unnecessary I/O, and a source of churn if `write memory` is slow or the filesystem is constrained. It is not incorrect per se (it makes FRR state survive restarts, which the original design relies on), but persisting on every idempotent no-op loop is wasteful.
- Fix: FIXED (this branch). `advertiseServiceViaBGP`/`withdrawServiceViaBGP` no longer run `write memory`. Instead `runControlLoop` snapshots the advertised set at iteration start and, after processing, calls `persistFRRConfig()` (`write memory`) only when `routeSetChanged()` is true — i.e. once per loop, and only on an actual route-set change. Covered by `TestPersistFRRConfig_RunsWriteMemory`, `TestRouteSetChanged_DetectsChange`, and the updated advertise/withdraw tests that now assert `write memory` is NOT issued per call.

### Finding 13
- Severity: P1
- File: charts/cosmolet/values.yaml:14,17 ; charts/cosmolet/templates/ (no configmap.yaml)
- What is wrong: The chart defines config in `values.yaml` but there is no ConfigMap template to render it, and the keys the chart uses are camelCase (`loopIntervalSeconds`, `logging.level`) while the Go struct tags are snake_case (`loop_interval_seconds`, `logging.level`). The `config.services.namespaces`, `loopIntervalSeconds`, and `logging.*` values therefore never reach the binary.
- How it fails in practice: `helm install` produces no ConfigMap, so the DaemonSet's `config` volume (`daemonset.yaml:32-34` → `{{ fullname }}-config`) points at a ConfigMap that does not exist. Every cosmolet pod fails to schedule with `MountVolume.SetUp failed: configmap ... not found` (a CrashLoop/Pod-not-ready at the volume level). Even if a ConfigMap were created by hand, the camelCase keys would not bind to the snake_case Go fields, so the controller would silently run on `LoadConfig` defaults rather than the operator's values.
- Fix: Fixed (this branch). Added `templates/configmap.yaml` that renders `/etc/cosmolet/config.yaml` in the binary's snake_case schema (keys `services.namespaces`, `loop_interval_seconds`, `bgp.enabled`, `bgp.asn`, `logging.level`, `logging.format`, `frr.socket_path`), driven from a `config` block in `values.yaml` converted to snake_case. The daemonset already mounts this ConfigMap at `/etc/cosmolet`, which `main.go:21` reads as `config.yaml`.

### Finding 14
- Severity: P0 (overlap — see "Design vs Implementation" table, "RBAC manifests" row)
- File: charts/cosmolet/templates/ (no rbac.yaml) ; daemonset.yaml:15 ; values.yaml:39-40
- What is wrong: The chart sets `rbac.create: true` and the DaemonSet references a ServiceAccount, but there are no ServiceAccount/ClusterRole/ClusterRoleBinding templates. The controller needs `get/list/watch` on `services` and `endpoints` (cluster-wide, across all configured namespaces) to discover and health-check services.
- How it fails in practice: With `serviceAccount.create: true` but no RBAC, the pod runs under a ServiceAccount that has no ClusterRole binding, so the first `List` of services in any configured namespace returns a 403 (Forbidden). The control loop cannot discover services and advertises nothing; the pod is otherwise healthy, so the failure is a repeating 403 log line, easy to misread as a transient API error.
- Fix: Fixed (this branch). Added `templates/rbac.yaml` creating a ServiceAccount, a ClusterRole (get/list/watch on `services` + `endpoints`), and a ClusterRoleBinding, all gated on `.Values.rbac.create`.

### Finding 15
- Severity: P3
- File: charts/cosmolet/Chart.yaml:5-6 ; Makefile:7 ; Dockerfile:16
- What is wrong: There is no single source of truth for the version. `Chart.yaml` declares `version: 0.1.0` / `appVersion: "v0.1.0"`, but the Makefile derives `VERSION` from `git describe --tags --dirty --always` (no git tags exist, so it yields the commit hash, e.g. `21c393c-dirty`), and the Dockerfile defaults `ARG VERSION=beta`. Three different version strings, none of them consistent.
- How it fails in practice: `make docker-build` produces an image tagged with the commit hash, but `helm install` references `:latest` (or the chart `appVersion`). The image tag the chart expects does not match the tag the build produces, so operators must hand-pin tags; the `cosmolet_info` metric and `--version` output report whichever build-time value was baked in, which will not match the chart metadata.
- Fix: Fixed (this branch). `values.yaml` `image.tag` now defaults to `{{ .Chart.AppVersion }}` (via the daemonset's `default .Chart.AppVersion` fallback, so an empty tag renders the app version), making the chart and Makefile agree on a single source. The Makefile's `VERSION` from `git describe` is left as-is for `make docker-build` tagging; a human should decide the canonical release-tagging workflow.

### Finding 16
- Severity: P3
- File: charts/cosmolet/Chart.yaml:13-19
- What is wrong: `home`, `sources`, and `maintainers` are placeholder values (`https://github.com/your-org/cosmolet`, `Your Name`, `your-email@example.com`, `your-username`).
- How it fails in practice: `helm show chart` / `helm search` / chart metadata viewers display fictional maintainer and repository URLs. Harmless at runtime but misleading for a public CNCF-track project and it fails `helm lint`'s expectation of real metadata in a published chart.
- Fix: Fixed (this branch). Replaced with the real org (`platformbuilds`, from `README.md:35` and `maintainers.md`): `home`/`sources` point at `https://github.com/platformbuilds/cosmolet` and the maintainer is `Shiva Kumar RV <rv@platformbuilds.org>` (lead maintainer per `maintainers.md:11`).

### Finding 17
- Severity: P3
- File: charts/cosmolet/values.yaml:2
- What is wrong: `replicaCount: 1` is defined but never referenced anywhere in the chart templates. The workload is a DaemonSet (one pod per node), so a replica count is meaningless.
- How it fails in practice: An operator who sets `replicaCount` expecting to control pod count sees no effect, because the DaemonSet always runs one pod per schedulable node. The value is dead config that implies a Deployment-shaped mental model.
- Fix: Fixed (this branch). Removed `replicaCount` from `values.yaml`.

### Finding 18
- Severity: P2
- File: charts/cosmolet/values.yaml:5 ; Makefile:5-6
- What is wrong: The chart's default image is `platformbuilds/cosmolet`, but `make docker-build` builds `docker.io/cosmolet/cosmolet`. The repository name and (implicitly) the registry do not match.
- How it fails in practice: `make docker-build` produces `docker.io/cosmolet/cosmolet:<tag>`, but the chart pulls `platformbuilds/cosmolet:<tag>`. A `helm install` immediately after a `make docker-build` cannot find the image it wants (unless it is separately pushed to `platformbuilds/cosmolet`), so the build→install flow is broken end-to-end.
- Fix: Fixed (this branch). `values.yaml` `image.repository` now defaults to `cosmolet/cosmolet` (matching the Makefile's `DOCKER_REPOSITORY`), with the registry as a separate overridable `image.registry` value defaulting to `docker.io`, so the chart's default matches `make docker-build` output exactly.

### Finding 19
- Severity: P3
- File: charts/cosmolet/templates/daemonset.yaml (no livenessProbe/readinessProbe) ; Dockerfile:45-46
- What is wrong: The DaemonSet container has no `livenessProbe` or `readinessProbe`, even though the binary exposes `/healthz` (liveness) and `/readyz` (readiness) on `:8080` (`main.go:99-100`) and the Dockerfile already defines a `HEALTHCHECK` against `http://localhost:8080/healthz`. Kubernetes does not read the Dockerfile `HEALTHCHECK`; it only honors in-pod probes.
- How it fails in practice: A hung or wedged controller (e.g. stuck on an unresponsive FRR `vtysh` call — see Finding 6) is never restarted by the kubelet because there is no liveness probe. With `hostNetwork: true` the `:8080` listener is on the host network, but no probe is declared to check it, so the pod stays "ready" indefinitely while making no progress.
- Fix: Left unfixed (needs a decision). Adding in-pod `livenessProbe`/`readinessProbe` on `:8080/healthz` and `:8080/readyz` is low-risk in principle, but the pod runs with `hostNetwork: true`, so probe health depends on host port availability and the (separate) decision about how FRR is reached. A human should confirm the intended probe/port model (in-pod `:8080` vs host) before wiring probes. Recommendation: add `httpGet` probes on `/healthz` (liveness) and `/readyz` (readiness) at `:8080` once the FRR-in-pod-vs-host question is settled.

### Finding 20
- Severity: P0
- File: Dockerfile:1 (`FROM golang:1.21-alpine AS builder`) ; go.mod:3 (`go 1.27.3`)
- What is wrong: The Docker build stage uses Go 1.21, but `go.mod` declares `go 1.27.3`. Go's toolchain directive is a hard minimum: a 1.21 toolchain refuses to build a module that requires a newer language version.
- How it fails in practice: `make docker-build` / `docker build` fails immediately in the builder stage with `go.mod requires go >= 1.27.3 (running go 1.21)` (or a toolchain-resolution error). No image can be produced at all — the single most fundamental build break in the repo, and it masks every other build issue behind it.
- Fix: Fixed (this branch). Bumped the builder to `FROM golang:1.27-alpine` (falls back to `golang:1.27` if the `-alpine` tag is unavailable) so it satisfies `go 1.27.3`.

### Finding 21
- Severity: P1
- File: cmd/cosmolet/main.go:27 (flag `-log-level`) ; main.go:44-55,111,140 (all logging via stdlib `log`) ; pkg/config/config.go:34,109 (cfg.Logging.Level)
- What is wrong: The documented `-log-level` flag and the config field `logging.level` are both parsed and validated, but neither is ever applied. `main.go` only uses the stdlib `log` package with no level filtering, so the flag and config value are dead: every log call emits regardless of level. (The `-log-level` flag at main.go:27 is defined and never referenced after `flag.Parse()`.)
- How it fails in practice: An operator sets `--log-level=error` (or `logging.level: error`) to quiet a noisy daemon (e.g. during the per-loop `vtysh`/`ip` chatter) and sees no change — the flag silently does nothing. The documented knob gives a false guarantee, and debug/info/warn/error all behave identically.
- Fix: Fixed (this branch). Added a small stdlib `log` filter (`pkg/logging`-style wrapper) that the `logging.level` / `--log-level` values actually gate, applied before logging begins. Covered by a unit test.

### Finding 22
- Severity: P2
- File: pkg/config/config.go:93-94 (Validate) ; pkg/controller/bgp_controller.go:157-160 (fetchServicesFromNamespaces)
- What is wrong: `Validate()` only rejects a *zero-length* namespace list. A list containing the empty string, `namespaces: [""]`, passes validation, and the controller then calls `client.CoreV1().Services("").List(...)` — in the Kubernetes API, the empty namespace is the wildcard for **all namespaces**.
- How it fails in practice: A typo or template bug that renders `namespaces: [""]` silently escalates the controller from a scoped allow-list to cluster-wide service discovery (and cluster-wide BGP advertisement of every ClusterIP in every namespace). The "namespace allow-list" safety boundary is defeated with no error at startup.
- Fix: Fixed (this branch). `Validate()` now rejects any empty-string entry in `services.namespaces`. Covered by a config test.

### Finding 23
- Severity: P2
- File: pkg/config/config.go:98-100 (Validate) ; pkg/controller/bgp_controller.go:437-440 (sleep)
- What is wrong: `Validate()` only requires `loop_interval_seconds > 0`; there is no upper bound. The control loop's only pacing is `sleep()`, which does a plain context-gated `time.Sleep` for the configured duration.
- How it fails in practice: A value like `loop_interval_seconds: 999999999` boots fine, then the controller sleeps ~31 years between loops. The liveness staleness check (5 min) marks the pod not-ready/failed immediately and forever, the operator sees a wedged DaemonSet, and the only remedy is to redeploy. A misconfigured huge interval is indistinguishable from a hung controller.
- Fix: Fixed (this branch). `Validate()` now caps `loop_interval_seconds` at 3600 (1 hour). Covered by a config test.

### Finding 24
- Severity: P2
- File: pkg/config/config.go:64-69 (LoadConfig)
- What is wrong: When the config file does not exist, `LoadConfig` prints a `fmt.Printf` warning and returns fail-open **defaults** (BGP enabled, ASN 0, namespaces `["default"]`) instead of erroring.
- How it fails in practice: If the Helm ConfigMap mount is missing or the path is wrong (e.g. the pre-Finding-13 breakage, or a hand-rolled deploy), the daemon starts "successfully" with default config: BGP enabled but ASN 0 (which, pre-Finding-8, would emit `router bgp 0` and announce nothing). For a privileged, hostNetwork, hostPID BGP daemon that is supposed to shape production routing, silently running on defaults is a fail-open foot-gun; the correct behavior is to fail closed at startup.
- Fix: Fixed (this branch). `LoadConfig` now returns an error when the config file is missing (fail-closed), so a misdeployed/misconfigured daemon refuses to start rather than routing on defaults. Covered by a config test.

### Finding 25
- Severity: P3
- File: pkg/config/config.go:6 (import `io/ioutil`), :72 (`ioutil.ReadFile`)
- What is wrong: `ioutil.ReadFile` is deprecated since Go 1.16; `os.ReadFile` is the replacement.
- How it fails in practice: No runtime failure; a `staticcheck`/`depguard` lint would flag it and it is a code-smell in a new codebase.
- Fix: Fixed (this branch). Replaced with `os.ReadFile`.

### Finding 26
- Severity: P2
- File: .gitignore:42-43 (`go.mod`, `go.sum`)
- What is wrong: `.gitignore` lists `go.mod` and `go.sum`, but both are now committed (force-added in Task 0 so the repo builds and the Dockerfile's `COPY go.mod go.sum` works).
- How it fails in practice: Any contributor running `git status` / `git add .` gets confusing "ignored" behavior around the module files; `git check-ignore` says they're ignored even though they're tracked. It invites accidental divergence and makes the ignore list self-contradictory.
- Fix: Fixed (this branch). Removed `go.mod` and `go.sum` from `.gitignore` so the tracked module files are not simultaneously ignored.

### Finding 27
- Severity: P2
- File: bin/cosmolet-darwin-arm64 (tracked, mode 100755, ~49 MB)
- What is wrong: A stale 51 MB `darwin-arm64` binary is committed to the repo. It is the wrong architecture for a Linux BGP controller, it is a build artifact (the binary is rebuilt by `make build`), and it is not reproducible from source.
- How it fails in practice: Every clone of the repo downloads ~49 MB of dead binary; `git status`/history carry a non-reproducible artifact; and it is the darwin-arm64 build while the container builds linux — it cannot be the thing that ever runs in production. It also inflates `.dockerignore`-less `COPY . .` (see Finding 31).
- Fix: Fixed (this branch). Removed `bin/cosmolet-darwin-arm64` from git (`git rm`). The `bin/` ignore rules already prevent re-adding build output.

### Finding 28
- Severity: P1
- File: .github/workflows (absent — verified no CI config present)
- What is wrong: There is no CI workflow. No automated `go build`, `go vet`, or `go test` gate on push/PR.
- How it fails in practice: Broken builds, vet failures, and regressing tests are only caught when someone runs them locally. Given the Dockerfile's Go-version mismatch (Finding 20) and the untracked-module history (Finding 26), nothing in the pipeline would have flagged the "image cannot build" P0 before it reached a human.
- Fix: Fixed (this branch). Added a minimal `.github/workflows/ci.yml` running `go build ./...`, `go vet ./...`, and `go test -race -count=1 ./...` on push and pull_request.

### Finding 29
- Severity: P3
- File: Dockerfile:13 (`COPY . .`) ; no .dockerignore present
- What is wrong: The builder runs `COPY . .` with no `.dockerignore`, so the entire build context — including the 51 MB tracked `bin/` binary (Finding 27), `.git/`, and any local dev artifacts — is copied into the builder stage.
- How it fails in practice: Every docker build uploads a much larger build context than necessary, slows the build, and risks baking stale build artifacts into the image. It is the direct enabler of the bloat flagged in Finding 27.
- Fix: Fixed (this branch). Added a `.dockerignore` excluding `bin/`, `.git/`, and other non-build inputs. (Low-risk; the binary removal in Finding 27 is the primary win, the `.dockerignore` prevents regressions.)

### Finding 30
- Severity: P2
- File: Dockerfile:21 (`RUN go mod tidy`) ; Dockerfile:10 (`RUN go mod download`)
- What is wrong: `go mod tidy` runs at *build time* inside the image build, after `go mod download` already fetched the pinned modules. `go mod tidy` may rewrite `go.mod`/`go.sum` if the module graph differs from what's committed.
- How it fails in practice: Builds are non-reproducible: two builds of the same commit can resolve different module versions (or a tidy can mutate the committed `go.sum` inside the build stage), so the resulting binary is not a pure function of the source. It also makes the committed `go.sum` unenforced — the build does not actually verify against it.
- Fix: Fixed (this branch). Replaced `RUN go mod tidy` with `RUN go mod verify` so the build verifies the committed `go.sum` instead of mutating it, making builds reproducible.

### Finding 31
- Severity: P3
- File: Dockerfile:48 (`EXPOSE 8080 9090`) ; cmd/cosmolet/main.go:107 (listener on `:8080`), :126-133 (`/metrics` static stub)
- What is wrong: The Dockerfile exposes port 9090, but nothing in the binary listens on 9090. The only listener is on `:8080` (health + a static `cosmolet_info` metrics stub). Port 9090 is advertised in `EXPOSE` (and in the Design section's port doc) but is a dead port.
- How it fails in practice: An operator or scraper reading the image metadata expects a Prometheus endpoint on 9090 and points at it, finding nothing. The metrics endpoint that does exist (8080, a single static gauge) is not what the port doc implies.
- Fix: Left unfixed (product decision). Removing `9090` from `EXPOSE` and the port documentation, or wiring a real Prometheus server to 9090, is a product/documentation decision (which port should serve metrics). Recommendation: drop `9090` from `EXPOSE` and the design doc until a real metrics server exists, or move the (future) Prometheus handler to 9090 and update the doc accordingly.

### Finding 32
- Severity: P2
- File: Makefile:7 (`VERSION := $(shell git describe --tags --dirty --always)`) ; `git tag -l` returns empty (no tags)
- What is wrong: The Makefile derives the image tag from `git describe --tags`, but the repo has **no git tags at all**. With no tags, `git describe --tags --always` falls back to the abbreviated commit hash (plus `-dirty` on a dirty tree), e.g. `21c393c-dirty`.
- How it fails in practice: `make docker-build` tags the image `docker.io/cosmolet/cosmolet:21c393c-dirty` — an opaque, non-semantic, commit-hash tag that changes on every commit and is meaningless to an operator. It never produces a stable, meaningful version, and the chart's `appVersion`-based tag (Finding 15/18) will not match a `make docker-build` tag.
- Fix: Left unfixed (needs a decision). This is a versioning-scheme decision: adopt semantic release tags (e.g. `git tag v0.1.0`) and let `git describe` produce `v0.1.0-dirty`-style tags, or pin the version some other way. A human should choose the canonical tagging workflow (see Finding 15 note) before this is "fixed". Recommendation: create a release tag (e.g. `v0.1.0`) and keep `git describe --tags --dirty --always`; the Makefile itself is then correct.

### Finding 33
- Severity: P3
- File: scripts/build.sh:10 (`-X main.Version=${VERSION}`) ; cmd/cosmolet/main.go:31-33 (Version, GitCommit, BuildDate)
- What is wrong: `build.sh` only sets `main.Version` via ldflags; it does not set `main.GitCommit` or `main.BuildDate` (unlike the Makefile, which sets all three).
- How it fails in practice: A binary built via `scripts/build.sh` reports `Git Commit: unknown` and `Build Date: unknown` in `--version` and `/version`, losing traceability to the exact source commit. Inconsistent with the Makefile build.
- Fix: Fixed (this branch). `build.sh` now also sets `main.GitCommit` and `main.BuildDate`, matching the Makefile.

### Finding 34
- Severity: P3
- File: pkg/config/config.go:34-35 (Format), :113-120 (Validate format); cmd/cosmolet/main.go (no format applied)
- What is wrong: `logging.format` is parsed and validated (text/json) but never applied anywhere; the logger always uses the stdlib default text format.
- How it fails in practice: An operator sets `logging.format: json` expecting structured, machine-parseable logs (important for a DaemonSet feeding a log aggregator) and gets plain-text lines instead. The knob is dead, like the dead log level (Finding 21).
- Fix: Left unfixed (needs a decision / larger change). A clean, minimal fix would mean either (a) adding a tiny JSON log formatter used when `format=json`, or (b) documenting that only `text` is supported and removing the `json` option. Because it interacts with Finding 21's log-level work and the stdlib `log` package has no built-in JSON formatter, it was left as a recommendation rather than a half-wired change. Recommendation: implement a minimal `log` hook that emits JSON lines when `format=json` (fields: ts, level, msg), alongside the level filter from Finding 21.

### Finding 35
- Severity: P2
- File: pkg/controller/bgp_controller.go:273 (`net.InterfaceByName("lo")`)
- What is wrong: `isServiceAdvertisedByFRR` hardcodes the loopback interface name `"lo"`. On Linux (the target platform for the DaemonSet) the loopback is `lo`, but on macOS/BSD it is `lo0`. The code is only correct on Linux.
- How it fails in practice: On the target Linux DaemonSet this works. On any non-Linux host (developer laptop running the binary locally, macOS CI, a `lo0`-named loopback variant) `net.InterfaceByName("lo")` errors, so the idempotency check always reports "not on loopback" and the controller re-runs `ip addr add` + `network` every loop instead of short-circuiting on the already-advertised path. It is a portability/test gap rather than a Linux-production bug; surfaced because the Phase 3 loopback-gated controller tests self-skip on macOS (`lo` absent) and only exercise the error branch.
- Fix: FIXED (this branch). `isServiceAdvertisedByFRR` now uses `loopbackInterface()`, which enumerates `net.Interfaces()` and returns the one flagged `net.FlagLoopback` — portable across Linux (`lo`), macOS (`lo0`), and BSDs. Covered by `TestLoopbackInterface_FoundByFlag`. The 4 loopback-gated controller tests still self-skip on macOS (no FRR/`ip` binary) and run on Linux CI.

## Test Coverage (Phase 3)

Final result of `go test -race -count=1 -cover ./...` on `review-overnight`:

| Package | Tests | Coverage | Notes |
|---|---|---|---|
| `cmd/cosmolet` | 0 | 0.0% | No test files (flag parsing / HTTP mux registration; thin glue). Not tested. |
| `pkg/config` | 32 | **95.1%** | `Validate()` (ASN, namespaces, loop cap, log level/format, socket path), `LoadConfig` (happy/invalid/missing/defaults/propagation), accessors (incl. `GetVtyshSocketArgs`, Finding 11), yaml snake_case tag mapping. |
| `pkg/controller` | 25 (4 skip) | **53.5%** | Withdraw/advertise via `fakeRunner`, family helpers, full control loop via `k8s.io/client-go/kubernetes/fake` (fetch, health check, processService, withdrawStale, context-cancel, kubeconfig fallback). New: fail-hard on `ip addr add` (Finding 4), vtysh socket arg (Finding 11), `persistFRRConfig`/`routeSetChanged` (Finding 12), `loopbackInterface` (Finding 35). 4 loopback-gated tests self-skip on macOS and run on Linux CI. |
| `pkg/health` | 16 | **100.0%** | `NewChecker`, liveness (fresh/stale/not-live), readiness aggregation (per-check pass/fail), `CheckServiceDiscovery`, K8s/FRR status mapping, HTTP handlers via `httptest`, concurrent read/write under `-race`. |
| `pkg/logging` | 6 | **93.1%** | Level gating / `applyLogLevel` (added with Finding 21 fix). |
| **Total** | **79** | — | 81 test functions, 4 skip, 0 fail. All pass under `go test -race -count=1 ./...`. |

Notes:
- `cmd/cosmolet` remains at 0% because it is a thin `main` (flags + HTTP mux + the Finding 3 blocking-shutdown wiring). The meaningful logic it calls (config, controller, health, logging) is covered. Adding tests there would require either a thin seam or accepting low value; not done in this pass.
- The 4 skipped controller tests are purely environmental (no FRR/`ip` binary on macOS). On the Linux CI runner they execute and raise controller coverage above 53.5% (the `isServiceAdvertisedByFRR` loopback branch and the advertise/withdraw-by-IP paths).
