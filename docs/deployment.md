
# Deployment Guide

## Fast path: generate configs
Use the interactive generator to produce Helm values & FRR configs:
```bash
chmod +x tools/gen-values.sh
./tools/gen-values.sh
helm upgrade --install cosmolet ./charts/cosmolet -n kube-system -f generated/helm-values.yaml
```

## Pick a topology
- Single ToR eBGP: [`examples/helm/values-single-tor.yaml`](examples/helm/values-single-tor.yaml)
- Dual ToR eBGP: [`examples/helm/values-dual-tor.yaml`](examples/helm/values-dual-tor.yaml)
- Route Reflector iBGP: [`examples/helm/values-route-reflector.yaml`](examples/helm/values-route-reflector.yaml)
- Dual-stack: [`examples/helm/values-dualstack.yaml`](examples/helm/values-dualstack.yaml)

## Verify
- Node FRR: `vtysh -c "show ip bgp summary"`
- Fabric FRR: check ECMP with the sample configs: [`examples/frr/tor-frr-ebgp.conf`](examples/frr/tor-frr-ebgp.conf), [`examples/frr/rr-frr-ibgp.conf`](examples/frr/rr-frr-ibgp.conf)
- Create a Service:
  - Local: [`examples/k8s/svc-lb-local.yaml`](examples/k8s/svc-lb-local.yaml)
  - Cluster: [`examples/k8s/svc-lb-cluster.yaml`](examples/k8s/svc-lb-cluster.yaml)
  - Dual-stack: [`examples/k8s/svc-dualstack-lb.yaml`](examples/k8s/svc-dualstack-lb.yaml)
