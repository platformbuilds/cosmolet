
# Compatibility
## Calico (BIRD) / Cilium (BGP)
- Can run alongside Cosmolet(FRR). Avoid overlapping advertisements for the same prefixes.
- Do not `redistribute connected/static` globally in FRR.

## Service Mesh (Istio)
- Egress gateways are orthogonal to Cosmolet. Cosmolet handles inbound VIPs only.

## kube-proxy (IPVS)
- For ClusterIP VIP announcements, run IPVS with `--ipvs-strict-arp=true` so the VIP is bound.
