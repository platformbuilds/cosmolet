
# Deployment Guide
## Prerequisites
- Kubernetes 1.22+
- FRR installed and running on each node (with BGP sessions to ToR/Core).
- ToR/Core configured for ECMP (see [frr-config.md](frr-config.md)).
- Helm.

## Install
```bash
helm upgrade --install cosmolet ./charts/cosmolet -n kube-system   --set config.bgp.asn=65001   --set securityContext.privileged=true
```

## Verify
1. **Pods running**:
   ```bash
   kubectl -n kube-system get ds cosmolet
   kubectl -n kube-system get pods -l app.kubernetes.io/name=cosmolet -o wide
   ```
2. **BGP sessions up** (on ToR/Core and node FRR):
   ```bash
   vtysh -c "show ip bgp summary"
   ```
3. **Create a Service** and confirm VIP is learned upstream:
   ```bash
   kubectl -n demo apply -f ../docs/examples/svc-nginx-lb-local.yaml
   vtysh -c "show ip bgp <VIP>"
   ```

## Upgrades
- Cosmolet is stateless; rolling updates are safe. See [operations.md](operations.md) for failure drills.
