package clientcache

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var clientCacheKeyTTL = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "capcdt",
		Subsystem: "client_cache",
		Name:      "access_key_ttl",
		Help:      "Access Key TTL of a client",
	},
	[]string{
		"key",
		"url",
		"organization",
	},
)

func (c *clientCacheProvider) RegisterMetrics() {
	metrics.Registry.MustRegister(clientCacheKeyTTL)
}
