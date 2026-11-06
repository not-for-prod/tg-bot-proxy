package chat //nolint:testpackage // Private constructors allow deterministic configuration tests.

import (
	"context"
	"testing"
	"time"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/entity"
	sharedworker "tg-bot-proxy/internal/pkg/worker"

	"github.com/prometheus/client_golang/prometheus"
)

func TestOutboxPublisherConstructor(t *testing.T) {
	t.Parallel()

	cfg := config.Instance()
	metrics := sharedworker.NewMetrics(prometheus.NewRegistry(), "test", "telegram_proxy")
	proxy := &fakeTelegramProxy{operations: []entity.Operation{{ID: 1}, {ID: 2}}}

	publisher := NewOutboxPublisher(proxy, metrics)
	if publisher.Name() != "telegram-outbox-publisher" || publisher.Interval() != cfg.Outbox.Interval {
		t.Fatalf("name=%q interval=%s", publisher.Name(), publisher.Interval())
	}

	publisher.Job(context.Background())
	if proxy.claimOperationsCalls < 2 || proxy.executeOperationCalls != 2 {
		t.Fatalf(
			"claim operations=%d execute operations=%d, want at least 2 claims and 2 executions",
			proxy.claimOperationsCalls,
			proxy.executeOperationCalls,
		)
	}
}

func TestOutboxPublisherRefillsFreeSlotBeforeSlowOperationCompletes(t *testing.T) {
	t.Parallel()

	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	thirdStarted := make(chan struct{})
	proxy := &fakeTelegramProxy{
		operations: []entity.Operation{{ID: 1}, {ID: 2}, {ID: 3}},
		execute: func(_ context.Context, operation entity.Operation) error {
			switch operation.ID {
			case 1:
				close(slowStarted)
				<-releaseSlow
			case 2:
				<-slowStarted
			case 3:
				close(thirdStarted)
			}

			return nil
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- runOutboxPublisher(context.Background(), 2, proxy)
	}()

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("slow operation did not start")
	}
	select {
	case <-thirdStarted:
	case <-time.After(time.Second):
		close(releaseSlow)
		t.Fatal("free executor slot was not refilled while the slow operation was in flight")
	}

	close(releaseSlow)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run outbox publisher: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("publisher did not finish after ready operations were drained")
	}

	proxy.mutex.Lock()
	defer proxy.mutex.Unlock()
	if proxy.maxActiveOperations != 2 {
		t.Fatalf("peak active operations=%d, want 2", proxy.maxActiveOperations)
	}
	if proxy.executeOperationCalls != 3 {
		t.Fatalf("execute operations=%d, want 3", proxy.executeOperationCalls)
	}
	if len(proxy.claimLimits) < 2 || proxy.claimLimits[0] != 2 || proxy.claimLimits[1] != 1 {
		t.Fatalf("claim limits=%v, want initial capacity 2 followed by refilled capacity 1", proxy.claimLimits)
	}
}
