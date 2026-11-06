package worker_test

import (
	"context"
	"errors"
	"testing"

	"tg-bot-proxy/internal/pkg/worker"

	"github.com/prometheus/client_golang/prometheus"
)

func TestJobPropagatesCancellation(t *testing.T) {
	t.Parallel()

	observed := make(chan error, 1)
	metrics := worker.NewMetrics(prometheus.NewRegistry(), "test", "worker")
	testWorker := worker.New(worker.Config{Name: "cancellation-test"}, func(ctx context.Context) error {
		<-ctx.Done()
		observed <- ctx.Err()
		return nil
	}, metrics)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		testWorker.Job(ctx)
		close(done)
	}()

	cancel()
	<-done
	if err := <-observed; !errors.Is(err, context.Canceled) {
		t.Fatalf("job context error=%v, want context.Canceled", err)
	}
}
