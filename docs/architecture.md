
# Architecture

**Per-node announcer** watches `Service`, `EndpointSlice`, and `Node` to decide if this node should advertise a VIP. Uses FRR `network`/`no network` to originate/withdraw.

- FRR is configured per-node and peers to ToR or RRs. See **FRR examples**: [single ToR](examples/frr/node-frr-single-tor.conf), [dual ToR](examples/frr/node-frr-dual-tor.conf), [iBGP RR](examples/frr/node-frr-rr.conf).
- Fabric side configs: [ToR eBGP](examples/frr/tor-frr-ebgp.conf), [RR iBGP](examples/frr/rr-frr-ibgp.conf).
