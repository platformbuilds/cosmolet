
# Deployment Guide

## Pick a topology and values
Use one of the prebuilt Helm values:
- Single ToR eBGP: [values-single-tor.yaml](examples/helm/values-single-tor.yaml)
- Dual ToR eBGP: [values-dual-tor.yaml](examples/helm/values-dual-tor.yaml)
- Route Reflector (iBGP): [values-route-reflector.yaml](examples/helm/values-route-reflector.yaml)
- Dual-stack: [values-dualstack.yaml](examples/helm/values-dualstack.yaml)

## Install
```bash
helm upgrade --install cosmolet ./charts/cosmolet -n kube-system -f docs/examples/helm/values-single-tor.yaml
```

## Post-install checks
- Node FRR: `vtysh -c "show ip bgp summary"`
- Fabric FRR: verify `maximum-paths` and ECMP; configs: [ToR](examples/frr/tor-frr-ebgp.conf), [RR](examples/frr/rr-frr-ibgp.conf).

## Create Services
- Local: [svc-lb-local.yaml](examples/k8s/svc-lb-local.yaml)
- Cluster: [svc-lb-cluster.yaml](examples/k8s/svc-lb-cluster.yaml)
- Dual-stack: [svc-dualstack-lb.yaml](examples/k8s/svc-dualstack-lb.yaml)
