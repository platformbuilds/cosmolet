
# Operations & Runbook
## Health
- `/healthz` and `/readyz` on port 8080.
- Prometheus metrics: see [metrics.md](metrics.md).

## Failure Drills
- **Node drain**: VIPs should withdraw from that node when local ready endpoints drop to 0 (eTP=Local).
- **FRR restart**: controller's periodic reconcile restores `network` lines.
- **Pod restart**: `preStop` withdraw is best‑effort; reconcile loop self‑heals.

## Rolling Upgrades
- DaemonSet rolling updates are safe. Observe VIP path counts on ToR/Core during rollout.
