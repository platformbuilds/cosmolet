#!/usr/bin/env bash
set -euo pipefail
BOLD="$(printf '\033[1m')"; RESET="$(printf '\033[0m')"
yesmode=false
for arg in "$@"; do case "$arg" in -y|--yes) yesmode=true;; esac; done

ask(){ local p="$1" d="$2" v; $yesmode && { echo "$d"; return; }; read -rp "$(printf '%b%s%b [%s]: ' "$BOLD" "$p" "$RESET" "$d")" v || true; echo "${v:-$d}"; }
ask_choice(){ local p="$1" d="$2" c="$3" v; while true; do v="$(ask "$p ($c)" "$d")"; case "$v" in single-tor|dual-tor|rr|local|cluster|none) echo "$v"; return;; *) $yesmode && { echo "$d"; return; }; echo "Invalid: $v" >&2;; esac; done; }

TOPOLOGY="${TOPOLOGY:-$(ask_choice 'Topology' 'single-tor' 'single-tor|dual-tor|rr')}"
NODE_ASN="${NODE_ASN:-$(ask 'Node ASN' '65101')}"
NEIGHBOR_IPS="${NEIGHBOR_IPS:-$(ask 'Neighbor IPs (comma-separated)' '10.0.0.1')}"
NEIGHBOR_ASNS="${NEIGHBOR_ASNS:-$(ask 'Neighbor ASNs (match count or single)' '65000')}"
DUAL_STACK="${DUAL_STACK:-$(ask 'Dual-stack (true/false)' 'true')}"
ENSURE_STATIC="${ENSURE_STATIC:-$(ask 'Ensure static Null0 before network? (true/false)' 'true')}"
IMAGE_REPO="${IMAGE_REPO:-$(ask 'Image repository' 'ghcr.io/platformbuilds/cosmolet')}"
IMAGE_TAG="${IMAGE_TAG:-$(ask 'Image tag' 'latest')}"
LOOP_INTERVAL="${LOOP_INTERVAL:-$(ask 'Reconcile loop interval (seconds)' '30')}"
NAMESPACE="${NAMESPACE:-$(ask 'Install namespace' 'kube-system')}"
OUT_DIR="${OUT_DIR:-$(ask 'Output directory' 'generated')}"
SERVICE_STYLE="${SERVICE_STYLE:-$(ask_choice 'Generate example Service' 'local' 'local|cluster|none')}"
SERVICE_NAMESPACE="${SERVICE_NAMESPACE:-$(ask 'Service namespace' 'demo')}"
SERVICE_NAME="${SERVICE_NAME:-$(ask 'Service name' 'web')}"

IFS=',' read -r -a nbr_ips <<< "$NEIGHBOR_IPS"
IFS=',' read -r -a nbr_asns <<< "$NEIGHBOR_ASNS"
if [ "${#nbr_asns[@]}" -eq 1 ] && [ "${#nbr_ips[@]}" -gt 1 ]; then
  a="${nbr_asns[0]}"; nbr_asns=(); for _ in "${nbr_ips[@]}"; do nbr_asns+=("$a"); done
fi
if [ "${#nbr_asns[@]}" -ne "${#nbr_ips[@]}" ]; then
  echo "NEIGHBOR_ASNS count must match NEIGHBOR_IPS (or provide a single ASN to reuse)" >&2; exit 1
fi

mkdir -p "$OUT_DIR"

cat > "${OUT_DIR}/helm-values.yaml" <<EOF
image:
  repository: ${IMAGE_REPO}
  tag: ${IMAGE_TAG}
  pullPolicy: IfNotPresent
config:
  loopIntervalSeconds: ${LOOP_INTERVAL}
  bgp:
    asn: ${NODE_ASN}
  frr:
    ensureStatic: ${ENSURE_STATIC}
securityContext:
  privileged: true
daemonset:
  hostNetwork: true
  hostPID: true
rbac:
  create: true
serviceAccount:
  create: true
  name: ""
EOF

{
  echo "frr version 8.5"
  echo "hostname NODE"
  echo "service integrated-vtysh-config"
  echo "router bgp ${NODE_ASN}"
  echo " bgp bestpath as-path multipath-relax"
  echo " maximum-paths 8"
  for i in "${!nbr_ips[@]}"; do
    echo " neighbor ${nbr_ips[$i]} remote-as ${nbr_asns[$i]}"
    echo " neighbor ${nbr_ips[$i]} timers 15 45"
  done
  echo "address-family ipv4 unicast"
  echo " exit-address-family"
  if [ "${DUAL_STACK}" = "true" ]; then
    echo "address-family ipv6 unicast"
    echo " exit-address-family"
  fi
  echo "line vty"
} > "${OUT_DIR}/node-frr.conf"

gen_fabric="N"; $yesmode || read -r -p "$(printf '%bGenerate fabric samples (ToR/RR)?%b [y/N]: ' "$BOLD" "$RESET")" gen_fabric || true; gen_fabric="${gen_fabric:-N}"
case "$gen_fabric" in y|Y)
  if [ "$TOPOLOGY" = "rr" ]; then
    cat > "${OUT_DIR}/rr-frr.conf" <<EOF
frr version 8.5
hostname RR
service integrated-vtysh-config
router bgp ${NODE_ASN}
 bgp cluster-id 10.0.1.254
 bgp bestpath as-path multipath-relax
 maximum-paths 64
 neighbor NODE-PEERS peer-group
 neighbor NODE-PEERS remote-as ${NODE_ASN}
 neighbor NODE-PEERS timers 15 45
# add node sessions as needed:
# neighbor <node-ip> peer-group NODE-PEERS
address-family ipv4 unicast
  neighbor NODE-PEERS route-reflector-client
 exit-address-family
$( [ "${DUAL_STACK}" = "true" ] && printf "address-family ipv6 unicast\n  neighbor NODE-PEERS route-reflector-client\n exit-address-family\n" )
line vty
EOF
  else
    cat > "${OUT_DIR}/tor-frr.conf" <<EOF
frr version 8.5
hostname TOR
service integrated-vtysh-config
router bgp ${nbr_asns[0]}
 bgp bestpath as-path multipath-relax
 maximum-paths 32
 neighbor NODE-PEERS peer-group
 neighbor NODE-PEERS timers 15 45
# Add node neighbors:
# neighbor <node-ip> peer-group NODE-PEERS
# neighbor <node-ip> remote-as ${NODE_ASN}
address-family ipv4 unicast
  neighbor NODE-PEERS route-map COSMO-IN in
  neighbor NODE-PEERS route-map COSMO-OUT out
 exit-address-family
$( [ "${DUAL_STACK}" = "true" ] && printf "address-family ipv6 unicast\n  neighbor NODE-PEERS route-map COSMO-IN in\n  neighbor NODE-PEERS route-map COSMO-OUT out\n exit-address-family\n" )
route-map COSMO-IN permit 10
 set local-preference 100
route-map COSMO-OUT permit 10
 set community ${nbr_asns[0]}:100 additive
line vty
EOF
  fi
  ;;
esac

if [ "$SERVICE_STYLE" != "none" ]; then
  etp="Local"; [ "$SERVICE_STYLE" = "cluster" ] && etp="Cluster"
  cat > "${OUT_DIR}/svc-${SERVICE_NAME}.yaml" <<EOF
apiVersion: v1
kind: Namespace
metadata: { name: ${SERVICE_NAMESPACE} }
---
apiVersion: v1
kind: Service
metadata:
  name: ${SERVICE_NAME}
  namespace: ${SERVICE_NAMESPACE}
spec:
  type: LoadBalancer
  externalTrafficPolicy: ${etp}
  selector: { app: ${SERVICE_NAME} }
  ports:
  - name: http
    port: 80
    targetPort: 8080
EOF
fi

printf "\\n%sDone!%s Wrote files in %s/\\n" "$BOLD" "$RESET" "$OUT_DIR"
