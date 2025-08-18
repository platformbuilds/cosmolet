
<h1 align="center">Cosmolet</h1>
<p align="center"><em>A bare‑metal Kubernetes BGP Service Controller — per‑node FRR announcements for Service VIPs with ECMP.</em></p>

---

## What is Cosmolet?
Cosmolet turns a Kubernetes cluster into a first‑class citizen of your datacenter network by **originating BGP routes for Service VIPs** (LoadBalancer — and optionally ClusterIP/IPVS) **from every node**. Upstream routers learn the VIP with **multiple equal‑cost paths**, enabling high availability and fast convergence **without** proprietary load balancers.

- **Per‑node announcer** (no leader)
- **Health‑gated**: respects `externalTrafficPolicy` and local endpoint readiness
- **FRR integration** on each node
- **Observability**: Prometheus metrics & health endpoints
- **Helm chart** for easy install

> Outbound (egress) traffic stays **Kubernetes/CNI‑managed**. Cosmolet is deliberately inbound‑only.

## Quick Links
- **Docs landing** → [`/docs/index.md`](docs/index.md)
- Concept & Architecture → [`/docs/overview.md`](docs/overview.md), [`/docs/architecture.md`](docs/architecture.md)
- Traffic Model (ingress/egress) → [`/docs/traffic.md`](docs/traffic.md)
- Deployment Guide → [`/docs/deployment.md`](docs/deployment.md)
- FRR & Fabric Settings → [`/docs/frr-config.md`](docs/frr-config.md)
- Helm Chart Docs → [`/docs/helm/quickstart.md`](docs/helm/quickstart.md), [`/docs/helm/values.md`](docs/helm/values.md)
- Operations & Runbook → [`/docs/operations.md`](docs/operations.md)
- Metrics & Monitoring → [`/docs/metrics.md`](docs/metrics.md), Alerts → [`/docs/alerts.md`](docs/alerts.md)
- Security → [`/docs/security.md`](docs/security.md)
- Troubleshooting → [`/docs/troubleshooting.md`](docs/troubleshooting.md)
- Migration (leader → per‑node) → [`/docs/migration.md`](docs/migration.md)
- Compatibility (Calico/Cilium/meshes) → [`/docs/compatibility.md`](docs/compatibility.md)
- Examples → [`/docs/examples/`](docs/examples/)

## Quick Start
```bash
helm upgrade --install cosmolet ./charts/cosmolet -n kube-system   --set config.bgp.asn=65001   --set securityContext.privileged=true
```

## Status
**Beta** — Functionally solid; continue scale/failure testing in your environment.

---

© 2025-08-18 Platformbuilds Inc.
