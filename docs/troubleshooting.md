
# Troubleshooting
### VIP not visible on ToR/Core
- Check FRR: `vtysh -c "show running-config"` → `network <VIP>` present?
- Ensure ECMP enabled and sessions are Established: `show ip bgp summary`.
- Verify eTP policy and local ready endpoints (`kubectl describe svc ...`).

### Traffic blackholes on Local
- Ensure local ready endpoints exist; otherwise announcement is suppressed.
- Confirm `EndpointSlice` labels match service name.

### Dual‑stack issues
- Enable `address-family ipv6 unicast` on both node FRR and ToR/Core.
