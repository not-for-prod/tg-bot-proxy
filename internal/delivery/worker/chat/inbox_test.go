package chat //nolint:testpackage // Private constructors allow deterministic configuration tests.

import (
	"context"
	"testing"

	"tg-bot-proxy/config"
	sharedworker "tg-bot-proxy/internal/pkg/worker"

	"github.com/prometheus/client_golang/prometheus"
)

func TestInboxProcessorConstructors(t *testing.T) {
	t.Parallel()

	cfg := config.Instance()
	metrics := sharedworker.NewMetrics(prometheus.NewRegistry(), "test", "telegram_proxy")

	proxy := &fakeTelegramProxy{}
	worker := NewInboxProcessor(proxy, metrics)
	if worker.Name() != "telegram-inbox" || worker.Interval() != cfg.Inbox.Interval {
		t.Fatalf("name=%q interval=%s", worker.Name(), worker.Interval())
	}

	worker.Job(context.Background())
	if proxy.inboxCalls != 1 {
		t.Fatalf("inbox calls=%d, want 1", proxy.inboxCalls)
	}
}
