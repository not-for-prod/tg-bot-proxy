package worker_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"tg-bot-proxy/internal/pkg/worker"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestWorkerMetricsRecordExecutionOutcomes(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	metrics := worker.NewMetrics(registry, "test", "worker")
	successful := worker.New(worker.Config{Name: "successful"}, func(context.Context) error { return nil }, metrics)
	failed := worker.New(
		worker.Config{Name: "failed"},
		func(context.Context) error { return errors.New("failed") },
		metrics,
	)

	successful.Job(context.Background())
	failed.Job(context.Background())

	expected := `
# HELP test_worker_worker_active_executions Current scheduled worker executions.
# TYPE test_worker_worker_active_executions gauge
test_worker_worker_active_executions{worker="failed"} 0
test_worker_worker_active_executions{worker="successful"} 0
# HELP test_worker_worker_executions_total Total scheduled worker executions by outcome.
# TYPE test_worker_worker_executions_total counter
test_worker_worker_executions_total{status="error",worker="failed"} 1
test_worker_worker_executions_total{status="success",worker="successful"} 1
`
	if err := testutil.GatherAndCompare(
		registry,
		strings.NewReader(expected),
		"test_worker_worker_active_executions",
		"test_worker_worker_executions_total",
	); err != nil {
		t.Fatal(err)
	}
}
