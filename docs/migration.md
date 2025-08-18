
# Migration: Leader → Per‑Node
This version removes leader election. Each node advertises VIPs independently.

## Steps
1. Upgrade helm release (new image/chart).
2. Verify FRR ECMP on nodes and ToR.
3. Validate with a test LoadBalancer Service (Local and Cluster policies).
4. Remove any legacy leader‑election RBAC/ConfigMaps (if present).
