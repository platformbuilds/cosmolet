
# FRR & Fabric Configuration

**Node FRR configs:**
- Single ToR: [node-frr-single-tor.conf](examples/frr/node-frr-single-tor.conf)
- Dual ToR: [node-frr-dual-tor.conf](examples/frr/node-frr-dual-tor.conf)
- iBGP to RRs: [node-frr-rr.conf](examples/frr/node-frr-rr.conf)

**Fabric configs:**
- ToR eBGP: [tor-frr-ebgp.conf](examples/frr/tor-frr-ebgp.conf)
- Route Reflector iBGP: [rr-frr-ibgp.conf](examples/frr/rr-frr-ibgp.conf)

**Guard rails:** Do not `redistribute connected/static` globally; Cosmolet injects explicit `network` lines only.
