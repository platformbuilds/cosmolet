
# Traffic Model (Ingress & Egress)

- **Ingress (Cosmolet)**: BGP advertise VIPs from each node for ECMP.
  - Try: [LB Local](examples/k8s/svc-lb-local.yaml) for strict locality, or [LB Cluster](examples/k8s/svc-lb-cluster.yaml) for ECMP everywhere.
  - Dual-stack: [svc-dualstack-lb.yaml](examples/k8s/svc-dualstack-lb.yaml).

- **Egress (CNI / kube-proxy / mesh)**: Out of scope for Cosmolet.
  - If advertising **ClusterIP** VIPs, enable IPVS strict ARP: [kube-proxy IPVS ConfigMap](examples/k8s/kube-proxy-ipvs-configmap.yaml).
