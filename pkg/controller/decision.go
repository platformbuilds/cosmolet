
package controller

import (
	"net"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

func ExtractVIPs(svc *corev1.Service) (v4 []net.IP, v6 []net.IP) {
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ip := net.ParseIP(ing.IP); ip != nil {
				if ip.To4() != nil { v4 = append(v4, ip) } else { v6 = append(v6, ip) }
			}
		}
	}
	ips := []string{}
	if svc.Spec.ClusterIP != "" { ips = append(ips, svc.Spec.ClusterIP) }
	ips = append(ips, svc.Spec.ClusterIPs...)
	for _, ipstr := range ips {
		if ipstr == "" || strings.ToLower(ipstr) == "none" { continue }
		if ip := net.ParseIP(ipstr); ip != nil {
			if ip.To4() != nil { v4 = append(v4, ip) } else { v6 = append(v6, ip) }
		}
	}
	uniq := func(in []net.IP) []net.IP {
		seen := map[string]bool{}; out := []net.IP{}
		for _, ip := range in { k := ip.String(); if !seen[k] { seen[k]=true; out=append(out, ip) } }
		sort.Slice(out, func(i,j int) bool { return strings.Compare(out[i].String(), out[j].String()) < 0 })
		return out
	}
	return uniq(v4), uniq(v6)
}

func localReadyEndpoints(nodeName string, slices []*discoveryv1.EndpointSlice, _ *corev1.Service) int {
	ready := 0
	for _, es := range slices {
		if es.AddressType != discoveryv1.AddressTypeIPv4 && es.AddressType != discoveryv1.AddressTypeIPv6 { continue }
		for _, ep := range es.Endpoints {
			if ep.NodeName == nil || *ep.NodeName != nodeName { continue }
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready { ready++ }
		}
	}
	return ready
}

type Policy string
const (
	PolicyAuto    Policy = "auto"
	PolicyLocal   Policy = "Local"
	PolicyCluster Policy = "Cluster"
)

func ShouldAdvertise(nodeName string, svc *corev1.Service, slices []*discoveryv1.EndpointSlice, p Policy, nodeSchedulable bool, nodeDraining bool, gateAnnotation *bool) bool {
	if gateAnnotation != nil && !*gateAnnotation { return false }
	if !nodeSchedulable || nodeDraining { return false }
	if !(svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeClusterIP) { return false }
	policy := p
	if policy == PolicyAuto {
		if svc.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal { policy = PolicyLocal } else { policy = PolicyCluster }
	}
	if policy == PolicyCluster { return true }
	return localReadyEndpoints(nodeName, slices, svc) > 0
}
