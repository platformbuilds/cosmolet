
package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldAdvertise_LocalPolicy(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
		},
	}
	node := "node-a"
	es := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{discoveryv1.LabelServiceName: "svc"}},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{{NodeName: &node, Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)}}},
	}
	ok := ShouldAdvertise(node, svc, []*discoveryv1.EndpointSlice{es}, PolicyAuto, true, false, nil)
	if !ok { t.Fatalf("expected advertise when local ready endpoint exists") }
}

func TestShouldAdvertise_LocalPolicy_NoLocalReady(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
		},
	}
	node := "node-a"
	es := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{discoveryv1.LabelServiceName: "svc"}},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{{NodeName: &node, Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(false)}}},
	}
	ok := ShouldAdvertise(node, svc, []*discoveryv1.EndpointSlice{es}, PolicyAuto, true, false, nil)
	if ok { t.Fatalf("expected NOT advertise when no local ready endpoint") }
}

func TestShouldAdvertise_ClusterPolicy(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
		},
	}
	node := "node-a"
	ok := ShouldAdvertise(node, svc, nil, PolicyAuto, true, false, nil)
	if !ok { t.Fatalf("expected advertise for Cluster policy on all nodes") }
}

func TestShouldAdvertise_AnnotationGate(t *testing.T) {
	gate := false
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns", Annotations: map[string]string{"cosmolet.platformbuilds.io/announce":"false"}},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
	}
	ok := ShouldAdvertise("node-a", svc, nil, PolicyAuto, true, false, &gate)
	if ok { t.Fatalf("expected annotation gate to disable advertise") }
}

func boolPtr(b bool)*bool { return &b }
