
# Security

- RBAC is limited to `get/list/watch` for `services`, `endpointslices`, `nodes`.
- DaemonSet runs privileged to access FRR. Consider NetworkPolicy: [allow-metrics.yaml](examples/networkpolicy/allow-metrics.yaml).
