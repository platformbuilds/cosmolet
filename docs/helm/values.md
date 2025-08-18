
# Helm Values Reference
```yaml
image:
  repository: ghcr.io/platformbuilds/cosmolet
  tag: latest
  pullPolicy: IfNotPresent

config:
  loopIntervalSeconds: 30
  bgp:
    asn: 65001
  frr:
    socketPath: /var/run/frr
    configPath: /etc/frr
    ensureStatic: true

securityContext:
  privileged: true

daemonset:
  hostNetwork: true
  hostPID: true
  nodeSelector: {}
  tolerations: []
```
- **config.loopIntervalSeconds** — reconcile tick; events also trigger reconcile.
- **config.bgp.asn** — local node ASN used in FRR `router bgp` stanza.
- **config.frr.ensureStatic** — inject/remove `Null0` static before `network`/`no network`.
- **securityContext.privileged** — required for FRR access (vtysh/socket).
