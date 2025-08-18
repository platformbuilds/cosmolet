
# Examples Index

## Helm values
- [helm/values-single-tor.yaml](helm/values-single-tor.yaml)
- [helm/values-dual-tor.yaml](helm/values-dual-tor.yaml)
- [helm/values-route-reflector.yaml](helm/values-route-reflector.yaml)
- [helm/values-dualstack.yaml](helm/values-dualstack.yaml)
- [helm/values-no-static.yaml](helm/values-no-static.yaml)

## FRR (nodes)
- [frr/node-frr-single-tor.conf](frr/node-frr-single-tor.conf)
- [frr/node-frr-dual-tor.conf](frr/node-frr-dual-tor.conf)
- [frr/node-frr-rr.conf](frr/node-frr-rr.conf)

## FRR (fabric)
- [frr/tor-frr-ebgp.conf](frr/tor-frr-ebgp.conf)
- [frr/rr-frr-ibgp.conf](frr/rr-frr-ibgp.conf)

## Kubernetes Services
- [k8s/svc-lb-local.yaml](k8s/svc-lb-local.yaml)
- [k8s/svc-lb-cluster.yaml](k8s/svc-lb-cluster.yaml)
- [k8s/svc-dualstack-lb.yaml](k8s/svc-dualstack-lb.yaml)
- [k8s/svc-clusterip-advertise.yaml](k8s/svc-clusterip-advertise.yaml)

## Kube-proxy IPVS (ClusterIP advertising)
- [k8s/kube-proxy-ipvs-configmap.yaml](k8s/kube-proxy-ipvs-configmap.yaml)

## Observability
- [monitoring/cosmolet-service.yaml](monitoring/cosmolet-service.yaml)
- [monitoring/servicemonitor.yaml](monitoring/servicemonitor.yaml)
- [monitoring/prometheus-rules.yaml](monitoring/prometheus-rules.yaml)

## NetworkPolicy
- [networkpolicy/allow-metrics.yaml](networkpolicy/allow-metrics.yaml)
