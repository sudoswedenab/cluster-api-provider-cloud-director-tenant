// Copyright 2025 Sudo Sweden AB
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clientcache

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type PrometheusMetrics struct {
	logger                  *slog.Logger
	registry                metrics.RegistererGatherer
	clientCacheKeyTTLMetric *prometheus.GaugeVec
	controllerClient        client.Client
	cache                   *clientCacheProvider
}

type PrometheusMetricsOption func(*PrometheusMetrics)

func WithLogger(logger *slog.Logger) PrometheusMetricsOption {
	return func(m *PrometheusMetrics) {
		m.logger = logger
	}
}

func WithPrometheusRegistry(registry metrics.RegistererGatherer) PrometheusMetricsOption {
	return func(m *PrometheusMetrics) {
		m.registry = registry
	}
}

func WithClientCache(cache *clientCacheProvider) PrometheusMetricsOption {
	return func(m *PrometheusMetrics) {
		m.cache = cache
	}
}

func WithManager(mgr ctrl.Manager) PrometheusMetricsOption {
	controllerClient := mgr.GetClient()

	return func(m *PrometheusMetrics) {
		m.controllerClient = controllerClient
	}
}

func NewPrometheusMetrics(prometheusMetricsOptions ...PrometheusMetricsOption) (*PrometheusMetrics, error) {
	clientCacheKeyTTL := prometheus.NewGaugeVec(
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
	m := PrometheusMetrics{
		clientCacheKeyTTLMetric: clientCacheKeyTTL,
	}
	for _, PrometheusMetricsOption := range prometheusMetricsOptions {
		PrometheusMetricsOption(&m)
	}

	m.registry.MustRegister(m.clientCacheKeyTTLMetric)

	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		buildMetric := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloud_director_tenant_build_info",
			},
			[]string{
				"goversion",
				"revision",
			},
		)

		revision := "(unknown)"
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" {
				revision = setting.Value
			}
		}

		labels := prometheus.Labels{
			"goversion": buildInfo.GoVersion,
			"revision":  revision,
		}

		buildMetric.With(labels).Inc()

		m.registry.MustRegister(buildMetric)
	}

	return &m, nil
}

func (m *PrometheusMetrics) CollectMetrics() error {
	ctx := context.Background()

	m.logger.Log(ctx, slog.LevelDebug-1, "collecting prometheus metrics")

	cache, err := m.cache.getCache()
	if err != nil {
		return err
	}

	m.clientCacheKeyTTLMetric.Reset()

	cacheKeys := cache.Keys()
	for _, key := range cacheKeys {
		i, found := cache.Get(key)
		if found {
			c := i.(*govcd.VCDClient)
			session, err := c.Client.GetSessionInfo()
			if err != nil {
				return err
			}

			labels := prometheus.Labels{
				"key":          fmt.Sprint(key),
				"url":          c.Client.VCDHREF.String(),
				"organization": session.Org.Name,
			}

			token := c.Client.VCDToken
			remainingTTL, err := getTokenRemainingTTL(token)
			if err != nil {
				return err
			}

			m.clientCacheKeyTTLMetric.With(labels).Set(remainingTTL.Seconds())
		}
	}

	return nil
}
