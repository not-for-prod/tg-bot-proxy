package main

import (
	"context"
	"os"
	"testing"
	"time"

	"tg-bot-proxy/config"
	chatservice "tg-bot-proxy/internal/application/service/chat"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/domain/service"
	"tg-bot-proxy/internal/pkg/worker"

	"github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
)

func TestMain(m *testing.M) {
	if err := os.Setenv("POSTGRES_DSN", "postgres://postgres@localhost:5432/tg_bot_proxy?sslmode=disable"); err != nil {
		panic(err)
	}
	if err := os.Setenv("TELEGRAM_BOT_TOKEN", "test-token"); err != nil {
		panic(err)
	}
	if err := os.Setenv("TELEGRAM_BOT_ID", "1"); err != nil {
		panic(err)
	}
	if err := os.Setenv("TELEGRAM_SUPERGROUP_ID", "-1001"); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestProvideWorkersReturnsThreeDistinctWorkers(t *testing.T) {
	t.Parallel()

	workers := provideWorkers(&graphTelegramProxy{}, testWorkerMetrics())
	if len(workers) != 3 {
		t.Fatalf("workers=%d, want 3", len(workers))
	}

	cfg := config.Instance()
	wantIntervals := map[string]time.Duration{
		"telegram-poller":           cfg.Telegram.Bot.Polling.Interval,
		"telegram-inbox":            cfg.Inbox.Interval,
		"telegram-outbox-publisher": cfg.Outbox.Interval,
	}
	names := make(map[string]struct{}, len(workers))
	for _, configuredWorker := range workers {
		if _, exists := names[configuredWorker.Name()]; exists {
			t.Fatalf("duplicate worker name %q", configuredWorker.Name())
		}
		names[configuredWorker.Name()] = struct{}{}
		if configuredWorker.Interval() != wantIntervals[configuredWorker.Name()] {
			t.Fatalf(
				"worker %q interval=%s, want %s",
				configuredWorker.Name(),
				configuredWorker.Interval(),
				wantIntervals[configuredWorker.Name()],
			)
		}
	}
}

func TestTelegramProxyWorkerGraph(t *testing.T) {
	t.Parallel()

	options := telegramProxyWorkerGraphOptions()
	if err := fx.ValidateApp(options...); err != nil {
		t.Fatalf("validate Fx graph: %v", err)
	}

	var workers []worker.Worker
	app := fx.New(append(options, fx.Populate(&workers))...)
	if err := app.Err(); err != nil {
		t.Fatalf("construct Fx graph: %v", err)
	}
	if len(workers) != 3 {
		t.Fatalf("constructed workers=%d, want 3", len(workers))
	}
}

func telegramProxyWorkerGraphOptions() []fx.Option {
	return []fx.Option{
		fx.Provide(
			testWorkerMetrics,
			testTelegramProxyMetrics,
			func() repository.Inbox { return &graphInboxRepository{} },
			func() repository.Chat { return &graphChatRepository{} },
			func() repository.Outbox { return &graphOutboxRepository{} },
			func() gateway.Telegram { return &graphTelegramGateway{} },
			func() trm.Manager { return graphTransactionManager{} },
			provideTelegramProxyService,
			provideWorkers,
		),
	}
}

func testWorkerMetrics() *worker.Metrics {
	return worker.NewMetrics(prometheus.NewRegistry(), "test", "telegram_proxy")
}

func testTelegramProxyMetrics() *chatservice.Metrics {
	return chatservice.NewMetrics(prometheus.NewRegistry(), "test", "telegram_proxy")
}

type graphInboxRepository struct {
	repository.Inbox
}

type graphChatRepository struct {
	repository.Chat
}

type graphOutboxRepository struct {
	repository.Outbox
}

type graphTelegramGateway struct {
	gateway.Telegram
}

type graphTelegramProxy struct {
	service.TelegramProxy
}

type graphTransactionManager struct{}

func (graphTransactionManager) Do(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (graphTransactionManager) DoWithSettings(
	ctx context.Context,
	_ trm.Settings,
	fn func(context.Context) error,
) error {
	return fn(ctx)
}
