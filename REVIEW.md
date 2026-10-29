# Cosmolet — Code Review Findings

> Unattended end-to-end code review of `main` @ 8ee3f9f, run on branch `review-overnight`.
> Severity: P0 critical / P1 high / P2 medium / P3 low.

## Executive Summary

*(written in Phase 4, step 11)*

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
| Leader election | "Ensures only one instance manages BGP routes while maintaining monitoring on all nodes" (README:17) | no | no `leader`, `Lease`, `resourcelock`, `election`, or coordination-lease code in any `.go` file; no `k8s.io/client-go/tools/leaderelection` import | Every DaemonSet pod runs the full control loop concurrently. Without leader election, multiple nodes will all attempt to advertise/withdraw the same routes. |
| Failover / high availability | "DaemonSet deployment with graceful failover" (README:18) | no | no failover logic, no route-withdrawal-on-shutdown, no re-election mechanism in code | On pod restart, the previous pod's FRR state (persisted via `write memory`) retains routes, but there is no coordinated handover. Multiple pods will duplicate work. |
| Prometheus metrics | "Prometheus metrics" (README:19); Dockerfile `EXPOSE 8080 9090` (Dockerfile:48) | partly | `main.go:126-133` defines `metricsHandler` that emits a single static `cosmolet_info` gauge with version/commit labels; no `prometheus/client_golang` import, no counter/histogram/gauge beyond that constant | The handler is a placeholder (comment says "basic for now"). Port 9090 is exposed in Dockerfile but nothing listens on it. No meaningful operational metrics (loop duration, services discovered, BGP success/failure counts). |
| Health checks (liveness/readiness) | Dockerfile HEALTHCHECK on `:8080/healthz` (Dockerfile:45-46); "Comprehensive Monitoring" (README:19) | yes | `main.go:99-101` registers `/healthz` (liveness) and `/readyz` (readiness); `checker.go:133-160` implements liveness (5-min staleness check), `:163-200` implements readiness (aggregates named checks); `bgp_controller.go:51-62` runs K8s API + FRR connectivity checks at startup | Functional. Liveness flags stale if no loop ran in 5 min. Readiness reports per-check status (kubernetes_api, frr_status, service_discovery). |
| RBAC manifests | "proper RBAC configuration" (README:20); `rbac.create: true` in values.yaml:39-41 | no | `charts/cosmolet/templates/` contains only `daemonset.yaml` and `_helpers.tpl`; no `rbac.yaml`, no ClusterRole/ClusterRoleBinding/Role/RoleBinding templates | The Helm flag exists but generates no resources. The DaemonSet references a ServiceAccount (`daemonset.yaml:15`) but no binding is created. |
| Security hardening / minimal privileges | "Minimal privileges with proper RBAC configuration" (README:20) | no | `values.yaml:42-44`: `privileged: true`, `runAsUser: 0`; `values.yaml:46-48`: `hostNetwork: true`, `hostPID: true`; `daemonset.yaml:36-37` mounts hostPath `/var/run/frr` | Container runs fully privileged as root with host network and PID namespaces. The "minimal privileges" claim is directly contradicted by the shipped chart. |
| Graceful shutdown | not explicitly documented as a feature (implied by "graceful failover" in README:18) | yes | `main.go:135-144` catches SIGINT/SIGTERM, calls `cancel()` on context, sleeps 5 s; `bgp_controller.go:64-72` and `:96-99` check `ctx.Done()` in loop and per-service processing; `:293-299` sleep is context-aware | Controller stops its loop cleanly. However, it does NOT withdraw BGP routes it previously advertised on shutdown (no `no network` / `clear bgp` cleanup). |
| Config persistence to FRR | not explicitly promised in docs (implied by production use) | yes | `bgp_controller.go:270-275` runs `vtysh -c "write memory"` after each advertisement, persisting to `/etc/frr/frr.conf` | Ensures FRR survives restarts. No corresponding un-persist on service removal (routes are never withdrawn). |

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
- Fix: Left unfixed. Implementing leader election requires adding Kubernetes coordination.k8s.io/Lease resources, a leader election loop, and coordinating between pods. This is a significant architectural change that should be reviewed by a human to determine the desired semantics (e.g., should only the leader advertise, or should all nodes advertise their local view?). A minimal fix would use `k8s.io/client-go/tools/leaderelection` with a Lease lock.

### Finding 3
- Severity: P1
- File: cmd/cosmolet/main.go:143-144
- What is wrong: On shutdown, the controller cancels the context and sleeps for 5 seconds, but does not explicitly wait for the BGP controller goroutine to finish route withdrawal.
- How it fails in practice: When the pod receives SIGTERM, `waitForShutdown` calls `cancel()` which signals the BGP controller to stop. The controller's `Start()` method then calls `withdrawAllRoutes()`, but the main goroutine only waits 5 seconds before the process exits. If route withdrawal takes longer than 5 seconds (e.g., many routes, slow FRR), the process may exit before all routes are withdrawn, leaving stale BGP routes.
- Fix: Left unfixed as part of the broader leader election/coordination design. A proper fix requires the main goroutine to wait for the controller goroutine to complete (e.g., using a channel or errgroup) rather than using a fixed sleep. This is related to Finding 2 and should be addressed together.

### Finding 4
- Severity: P2
- File: pkg/controller/bgp_controller.go:271-274
- What is wrong: When `ip addr add` fails, the code logs a warning and continues to advertise the route via BGP. This can result in advertising a route for an IP that is not actually assigned to the local interface.
- How it fails in practice: If the kernel rejects the IP assignment (e.g., IP already exists, permission denied, interface not ready), FRR will still advertise the route. Other nodes will try to send traffic to this node for that ClusterIP, but the node cannot receive it because the IP is not assigned, causing packet drops.
- Fix: Left unfixed. The fix would be to return an error from `advertiseServiceViaBGP` if `ip addr add` fails, preventing the BGP advertisement. However, this changes behavior and may mask transient kernel errors. A human should decide whether to fail hard or retry.

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
- Fix: Left unfixed. The fix would be to use `exec.CommandContext` with a timeout (e.g., 10s) for all vtysh and ip commands. This is a straightforward change but was not prioritized for this pass.
