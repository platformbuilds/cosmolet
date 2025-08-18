
# FRR & Fabric Configuration
On **each node** running FRR:
- Enable ECMP:
  ```
  router bgp <ASN>
   bgp bestpath as-path multipath-relax
   maximum-paths 8
  ```
- Do **not** globally `redistribute connected` or `static`. Cosmolet injects explicit `network` statements.

On **ToR/Core**:
- Peer with all nodes (or via route reflectors).
- Enable ECMP for VIP prefixes and set policies to keep attributes (LocalPref/MED) uniform.

Cosmolet can optionally place a **static Null0** route before `network` to ensure origination even if the VIP is not locally bound.
