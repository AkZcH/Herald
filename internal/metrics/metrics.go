// Package metrics defines Herald's Prometheus metrics and the /metrics handler.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Sources are the callbacks the gauges read at scrape time.
// A nil source reports 0.
type Sources struct {
	RaftTerm    func() float64
	CommitIndex func() float64
	StaleAgents func() float64
}

// Metrics owns a private registry, so creating more than one (e.g. in tests)
// never collides on the global default registry.
type Metrics struct {
	registry *prometheus.Registry

	PropagationLatencyMs  prometheus.Histogram
	RollbackTotal         prometheus.Counter
	DeliveryFailuresTotal prometheus.Counter
}

func New(src Sources) *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		PropagationLatencyMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "herald_propagation_latency_ms",
			Help:    "Time from receiving a config write or rollback to the push being fanned out to agents, in milliseconds.",
			Buckets: []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000},
		}),
		RollbackTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "herald_rollback_total",
			Help: "Total number of completed rollbacks.",
		}),
		DeliveryFailuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "herald_delivery_failures_total",
			Help: "Total number of config pushes dropped because an agent's send buffer was full.",
		}),
	}

	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.PropagationLatencyMs,
		m.RollbackTotal,
		m.DeliveryFailuresTotal,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "herald_raft_term",
			Help: "Current raft term as seen by this node.",
		}, orZero(src.RaftTerm)),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "herald_raft_commit_index",
			Help: "Highest raft log index whose config version is COMMITTED.",
		}, orZero(src.CommitIndex)),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "herald_stale_agents",
			Help: "Number of agents currently in STALE state.",
		}, orZero(src.StaleAgents)),
	)

	return m
}

// Handler serves the Prometheus text exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func orZero(f func() float64) func() float64 {
	if f == nil {
		return func() float64 { return 0 }
	}
	return f
}