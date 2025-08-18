
# Traffic Model (Ingress & Egress)
## Inbound (Cosmolet's Scope)
- Originates BGP routes for Service VIPs from each node.
- ECMP upstream enables scale and resilience.
- Health‑gated per `eTP` and local readiness.

## Outbound (Kubernetes/CNI)
- Handled by your CNI, kube‑proxy (IPVS/iptables), or service mesh.
- Common patterns:
  - Node SNAT (default)
  - Cilium Egress Gateway
  - Calico Egress / Calico BGP
  - Mesh Egress Gateway

## Symmetry Considerations
- **eTP=Local**: preserves locality and symmetric returns.
- **eTP=Cluster**: may lead to asymmetric return paths; ensure NAT/routing tolerates this or prefer Local.

## Dual‑Stack
- VIPs are advertised independently as `/32` (IPv4) and `/128` (IPv6).
- Ensure `address-family ipv6 unicast` is enabled in FRR and ToR.
