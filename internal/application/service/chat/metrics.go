package chat

import (
	"tg-bot-proxy/internal/domain/entity"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	operationDispositionCompleted = "completed"
	operationDispositionRetry     = "retry"
	operationDispositionFailed    = "failed"
	inboxDispositionFailed        = "failed"
)

type Metrics struct {
	outboxDispositions *prometheus.CounterVec
	inboxDispositions  *prometheus.CounterVec
}

func NewMetrics(registerer prometheus.Registerer, namespace, subsystem string) *Metrics {
	metrics := &Metrics{
		outboxDispositions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "outbox_operation_dispositions_total",
				Help:      "Total durable outbox operation dispositions.",
			},
			[]string{"operation_type", "disposition"},
		),
		inboxDispositions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "inbox_item_dispositions_total",
				Help:      "Total durable inbox item dispositions.",
			},
			[]string{"disposition"},
		),
	}

	registerer.MustRegister(metrics.outboxDispositions, metrics.inboxDispositions)

	return metrics
}

func (m *Metrics) recordOutboxDisposition(operationType entity.OperationType, disposition string) {
	m.outboxDispositions.WithLabelValues(string(operationType), disposition).Inc()
}
