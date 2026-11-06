package chat //nolint:testpackage // Private constructors allow deterministic configuration tests.

import (
	"context"
	"testing"
	"tg-bot-proxy/config"
	sharedworker "tg-bot-proxy/internal/pkg/worker"

	"github.com/prometheus/client_golang/prometheus"
)

func TestPollerConstructors(t *testing.T) {
	t.Parallel()

	cfg := config.Instance()
	metrics := sharedworker.NewMetrics(prometheus.NewRegistry(), "test", "telegram_proxy")

	proxy := &fakeTelegramProxy{}
	worker := NewPoller(proxy, metrics)
	if worker.Name() != "telegram-poller" || worker.Interval() != cfg.Telegram.Bot.Polling.Interval {
		t.Fatalf("name=%q interval=%s", worker.Name(), worker.Interval())
	}

	worker.Job(context.Background())
	if proxy.pollCalls != 1 {
		t.Fatalf("poll calls=%d, want 1", proxy.pollCalls)
	}
}
