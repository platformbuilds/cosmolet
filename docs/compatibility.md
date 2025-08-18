
# Compatibility

- Calico/Cilium BGP can coexist. Avoid overlapping advertisements; keep FRR from `redistribute connected/static` globally.
- Advertising ClusterIP? Enable IPVS strict ARP: [kube-proxy-ipvs-configmap.yaml](examples/k8s/kube-proxy-ipvs-configmap.yaml).
