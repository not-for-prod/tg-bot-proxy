package worker

import "github.com/prometheus/client_golang/prometheus"

const (
	workerStatusSuccess = "success"
	workerStatusError   = "error"
)

type Metrics struct {
	executions *prometheus.CounterVec
	duration   *prometheus.HistogramVec
	active     *prometheus.GaugeVec
}

func NewMetrics(registerer prometheus.Registerer, namespace, subsystem string) *Metrics {
	metrics := &Metrics{
		executions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "worker_executions_total",
				Help:      "Total scheduled worker executions by outcome.",
			},
			[]string{"worker", "status"},
		),
		duration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "worker_execution_duration_seconds",
				Help:      "Scheduled worker execution duration in seconds.",
			},
			[]string{"worker"},
		),
		active: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "worker_active_executions",
				Help:      "Current scheduled worker executions.",
			},
			[]string{"worker"},
		),
	}

	registerer.MustRegister(metrics.executions, metrics.duration, metrics.active)

	return metrics
}
