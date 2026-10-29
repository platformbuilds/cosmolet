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

*(written in Phase 1, step 2)*

## Findings
