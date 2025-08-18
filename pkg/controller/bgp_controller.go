package controller

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1core "k8s.io/client-go/informers/core/v1"
	v1disc "k8s.io/client-go/informers/discovery/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/platformbuilds/cosmolet/pkg/frr"
	"github.com/platformbuilds/cosmolet/pkg/metrics"
)

type Config struct {
	NodeName              string
	LoopInterval          time.Duration
	ServiceInformer       v1core.ServiceInformer
	EndpointSliceInformer v1disc.EndpointSliceInformer
	NodeInformer          v1core.NodeInformer
	KubeClient            *kubernetes.Clientset
	ASN                   int
	EnsureStatic          bool
	VTYSHPath             string
}

type controller struct {
	cfg       Config
	frr       frr.Manager
	mu        sync.Mutex
	desired   map[string]bool
	announced map[string]bool
}

func NewBGPController(cfg Config) (*controller, error) {
	asn := cfg.ASN
	if asn == 0 {
		asn = 65001
	}
	mgr := frr.NewVTYSH(frr.Config{ASN: asn, EnsureStatic: cfg.EnsureStatic, VTYSHPath: cfg.VTYSHPath})

	c := &controller{
		cfg:       cfg,
		frr:       mgr,
		desired:   map[string]bool{},
		announced: map[string]bool{},
	}

	// Service events
	if _, err := cfg.ServiceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(interface{}) { go c.ReconcileOnce(context.Background()) },
		UpdateFunc: func(interface{}, interface{}) {
			go c.ReconcileOnce(context.Background())
		},
		DeleteFunc: func(interface{}) { go c.ReconcileOnce(context.Background()) },
	}); err != nil {
		return nil, fmt.Errorf("add service event handler: %w", err)
	}

	// EndpointSlice events
	if _, err := cfg.EndpointSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(interface{}) { go c.ReconcileOnce(context.Background()) },
		UpdateFunc: func(interface{}, interface{}) {
			go c.ReconcileOnce(context.Background())
		},
		DeleteFunc: func(interface{}) { go c.ReconcileOnce(context.Background()) },
	}); err != nil {
		return nil, fmt.Errorf("add endpointslice event handler: %w", err)
	}

	// Node events
	if _, err := cfg.NodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(interface{}) { go c.ReconcileOnce(context.Background()) },
		UpdateFunc: func(interface{}, interface{}) {
			go c.ReconcileOnce(context.Background())
		},
		DeleteFunc: func(interface{}) { go c.ReconcileOnce(context.Background()) },
	}); err != nil {
		return nil, fmt.Errorf("add node event handler: %w", err)
	}

	return c, nil
}

func (c *controller) Run(ctx context.Context) {
	t := time.NewTicker(c.cfg.LoopInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.ReconcileOnce(ctx)
		}
	}
}

func (c *controller) WithdrawAll(ctx context.Context) {
	c.mu.Lock()
	keys := make([]string, 0, len(c.announced))
	for k := range c.announced {
		keys = append(keys, k)
	}
	c.mu.Unlock()

	for _, k := range keys {
		ip, pfx := parseCIDRKey(k)
		if err := c.frr.WithdrawVIP(ip, pfx); err != nil {
			log.Printf("withdraw %s error: %v", k, err)
		} else {
			metrics.VIPWithdrawn.WithLabelValues("*", "*", ipFamily(ip), c.cfg.NodeName).Inc()
		}
	}
}

func (c *controller) ReconcileOnce(ctx context.Context) {
	// Proper panic guard (don't ignore recover result)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in ReconcileOnce: %v", r)
			// If you define a metric for panics, bump it here:
			// metrics.ReconcilePanics.Inc()
		}
	}()

	c.computeDesired(ctx)
	c.apply(ctx)
}

func (c *controller) computeDesired(ctx context.Context) {
	desired := map[string]bool{}

	svcs, _ := c.cfg.ServiceInformer.Lister().List(labels.Everything())
	node, _ := c.cfg.NodeInformer.Lister().Get(c.cfg.NodeName)
	if node == nil {
		return
	}

	nodeSched := !node.Spec.Unschedulable
	nodeDraining := false
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionTrue {
			nodeDraining = true
		}
	}

	for _, svc := range svcs {
		var annGate *bool
		if v, ok := svc.Annotations["cosmolet.platformbuilds.io/announce"]; ok {
			b := strings.ToLower(v) == "true"
			annGate = &b
		}

		eps, _ := c.cfg.EndpointSliceInformer.Lister().EndpointSlices(svc.Namespace).List(labels.Everything())
		esForSvc := []*discoveryv1.EndpointSlice{}
		for _, es := range eps {
			if es.Labels[discoveryv1.LabelServiceName] == svc.Name {
				esForSvc = append(esForSvc, es)
			}
		}

		policy := PolicyAuto
		if ShouldAdvertise(c.cfg.NodeName, svc, esForSvc, policy, nodeSched, nodeDraining, annGate) {
			v4, v6 := ExtractVIPs(svc)
			for _, ip := range v4 {
				desired[frr.Key(ip, 32)] = true
			}
			for _, ip := range v6 {
				desired[frr.Key(ip, 128)] = true
			}
			metrics.EndpointsReady.WithLabelValues(svc.Name, svc.Namespace, c.cfg.NodeName).
				Set(float64(localReadyEndpoints(c.cfg.NodeName, esForSvc, svc)))
		} else {
			metrics.EndpointsReady.WithLabelValues(svc.Name, svc.Namespace, c.cfg.NodeName).Set(0)
		}
	}

	c.mu.Lock()
	c.desired = desired
	c.mu.Unlock()
}

func ipFamily(ip net.IP) string {
	if ip.To4() != nil {
		return "ipv4"
	}
	return "ipv6"
}

func parseCIDRKey(k string) (net.IP, int) {
	parts := strings.Split(k, "/")
	pfx := 32
	if strings.Contains(parts[0], ":") {
		pfx = 128
	}
	if len(parts) == 2 {
		if n, err := strconv.Atoi(parts[1]); err == nil {
			pfx = n
		}
	}
	return net.ParseIP(parts[0]), pfx
}

func (c *controller) apply(ctx context.Context) {
	c.mu.Lock()
	desired := map[string]bool{}
	for k, v := range c.desired {
		desired[k] = v
	}
	announced := map[string]bool{}
	for k, v := range c.announced {
		announced[k] = v
	}
	c.mu.Unlock()

	for k := range desired {
		if !announced[k] {
			ip, pfx := parseCIDRKey(k)
			if err := c.frr.AnnounceVIP(ip, pfx); err != nil {
				metrics.ReconcileErrors.Inc()
				log.Printf("announce %s failed: %v", k, err)
				continue
			}
			metrics.VIPAdvertised.WithLabelValues("*", "*", ipFamily(ip), c.cfg.NodeName).Inc()
			announced[k] = true
		}
	}

	for k := range announced {
		if !desired[k] {
			ip, pfx := parseCIDRKey(k)
			if err := c.frr.WithdrawVIP(ip, pfx); err != nil {
				metrics.ReconcileErrors.Inc()
				log.Printf("withdraw %s failed: %v", k, err)
				continue
			}
			metrics.VIPWithdrawn.WithLabelValues("*", "*", ipFamily(ip), c.cfg.NodeName).Inc()
			delete(announced, k)
		}
	}

	c.mu.Lock()
	c.announced = announced
	c.mu.Unlock()
}
