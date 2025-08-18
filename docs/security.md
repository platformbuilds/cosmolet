
# Security
- **Privileges**: DaemonSet runs privileged to access FRR (`/var/run/frr`, `/etc/frr`).
- **RBAC**: `get/list/watch` for `services`, `endpointslices`, `nodes`.
- **NetworkPolicies**: allow Prometheus to scrape `:8080` if policies are enforced.
- **Least Privilege**: avoid FRR `redistribute` commands; Cosmolet injects only `network`/`no network`.
