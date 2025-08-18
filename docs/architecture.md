
# Architecture
## Components
- **DaemonSet pod (per node)**: Watches `Service`, `EndpointSlice`, `Node` and decides node‑local VIP advertisements.
- **FRR (per node)**: Receives `network` origination commands from Cosmolet (via `vtysh`).
- **Upstream fabric**: iBGP/eBGP to ToR/Core; ECMP across nodes for the same VIP.

## Decision Engine
- `externalTrafficPolicy: Local` → advertise **only** on nodes with ≥1 *local* ready endpoint.
- `externalTrafficPolicy: Cluster` → advertise on **all** nodes.
- Optional annotation gate: `cosmolet.platformbuilds.io/announce: "true|false"`.
- Node gates: unschedulable / network‑unavailable → do not advertise.

## Reconciliation
1. Compute **desired** VIP set for this node from informers.
2. Diff against **announced** VIP set.
3. Issue FRR `network`/`no network` (and optional static route) to converge.

## Metrics
See [metrics.md](metrics.md).
