
# Overview

Cosmolet advertises Kubernetes **Service VIPs** to your fabric via **BGP**, using **FRR on every node** for ECMP. No leader election; each DaemonSet pod acts independently.

- See **example Services**: [LB Local](examples/k8s/svc-lb-local.yaml), [LB Cluster](examples/k8s/svc-lb-cluster.yaml), [Dual-stack LB](examples/k8s/svc-dualstack-lb.yaml).
