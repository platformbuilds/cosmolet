
# Overview
**Cosmolet** is a Kubernetes controller/agent that **advertises Service IPs** (VIPs) to your datacenter fabric via **BGP** using **FRR** on every node. This enables **ECMP** at the ToR/Core and removes the need for external load‑balancer appliances in bare‑metal clusters.

### Key Capabilities
- Per‑node VIP origination (no leader election).
- Health‑gated announcements based on `externalTrafficPolicy` and *local* ready endpoints.
- Dual‑stack (IPv4/IPv6) VIP advertising (`/32`, `/128`).
- Idempotent reconciliation with FRR (`network` lines with optional static `Null0` origination).
- Prometheus metrics and health probes.

### Non‑Goals
- Managing **egress** paths, NAT, or pod routing (leave to CNI/mesh).
- Advertising pod CIDRs (use your CNI if desired).
- Replacing Calico/Cilium BGP; Cosmolet can **coexist** with them.
