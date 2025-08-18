
# Q&A Config Generator

Use `tools/gen-values.sh` to generate **Helm values** and **FRR configs** interactively.

## Usage
```bash
chmod +x tools/gen-values.sh
./tools/gen-values.sh
```

### Non-interactive (CI-friendly)
```bash
TOPOLOGY=dual-tor NODE_ASN=65101 NEIGHBOR_IPS=10.0.0.1,10.0.0.2 NEIGHBOR_ASNS=65000 \
DUAL_STACK=true ENSURE_STATIC=true IMAGE_REPO=ghcr.io/platformbuilds/cosmolet IMAGE_TAG=latest \
LOOP_INTERVAL=20 NAMESPACE=kube-system OUT_DIR=generated SERVICE_STYLE=local \
SERVICE_NAMESPACE=demo SERVICE_NAME=web \
./tools/gen-values.sh --yes
```

**Outputs (default `./generated/`):**
- `helm-values.yaml` — pass with `-f` to `helm upgrade --install`
- `node-frr.conf` — install/merge into `/etc/frr/frr.conf`
- Optional fabric: `tor-frr.conf` or `rr-frr.conf`
- Optional: `svc-<name>.yaml` sample Service
