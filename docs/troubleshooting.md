
# Troubleshooting

- VIP not on fabric? Compare against FRR configs: [node FRR](examples/frr/node-frr-single-tor.conf) and create a sample Service: [svc-lb-local.yaml](examples/k8s/svc-lb-local.yaml).
- Dual-stack? Use [svc-dualstack-lb.yaml](examples/k8s/svc-dualstack-lb.yaml) and ensure v6 AF enabled in FRR (node + ToR).
