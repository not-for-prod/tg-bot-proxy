package config //nolint:testpackage // parseConfig is the behavior under test.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestParseConfigTelegramProxy(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://postgres-env/test")
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("TELEGRAM_BOT_ID", "42")
	t.Setenv("TELEGRAM_SUPERGROUP_ID", "-10042")
	v, err := loadTestConfig(t)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := parseConfig(v)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.Bot.Polling.Timeout != 30*time.Second {
		t.Fatalf("unexpected polling timeout: %s", cfg.Telegram.Bot.Polling.Timeout)
	}
	if cfg.Outbox.WorkerPoolSize != 10 {
		t.Fatalf("unexpected worker pool size: %d", cfg.Outbox.WorkerPoolSize)
	}
	if cfg.Postgres.DSN != "postgres://postgres-env/test" {
		t.Fatal("PostgreSQL DSN was not loaded from the environment")
	}
	if cfg.Telegram.Bot.Token != "test-token" {
		t.Fatal("Telegram bot token was not loaded from the environment")
	}
	if cfg.Telegram.Bot.ID != 42 {
		t.Fatal("Telegram bot ID was not loaded from the environment")
	}
	if cfg.Telegram.SupergroupID != -10042 {
		t.Fatal("Telegram supergroup ID was not loaded from the environment")
	}
}

func TestParseConfigRejectsInvalidRetry(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	v, err := loadTestConfig(t)
	if err != nil {
		t.Fatal(err)
	}
	v.Set("outbox.retry.maxAttempts", 0)

	if _, parseErr := parseConfig(v); parseErr == nil {
		t.Fatal("expected invalid retry configuration to fail")
	}
}

func TestParseConfigCapsWorkSizes(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	tests := []struct {
		name  string
		key   string
		value int
	}{
		{name: "inbox batch", key: "inbox.batchSize", value: 1001},
		{name: "outbox worker pool", key: "outbox.workerPoolSize", value: 101},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, err := loadTestConfig(t)
			if err != nil {
				t.Fatal(err)
			}
			v.Set(test.key, test.value)

			if _, parseErr := parseConfig(v); parseErr == nil {
				t.Fatal("expected oversized work configuration to fail")
			}
		})
	}
}

func TestParseConfigValidatesOutboxLeaseDuration(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "one nanosecond", value: "1ns", wantErr: true},
		{name: "two nanoseconds", value: "2ns", wantErr: true},
		{name: "below sensible minimum", value: "999ms", wantErr: true},
		{name: "minimum", value: "1s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, err := loadTestConfig(t)
			if err != nil {
				t.Fatal(err)
			}
			v.Set("outbox.leaseDuration", test.value)

			_, parseErr := parseConfig(v)
			if test.wantErr && parseErr == nil {
				t.Fatal("expected invalid outbox lease duration to fail")
			}
			if !test.wantErr && parseErr != nil {
				t.Fatalf("expected valid outbox lease duration, got %v", parseErr)
			}
		})
	}
}

func TestParseConfigValidatesStartMessage(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "required", value: "", wantErr: true},
		{name: "blank", value: " \n\t", wantErr: true},
		{name: "4096 ASCII characters", value: strings.Repeat("a", 4096)},
		{name: "4097 ASCII characters", value: strings.Repeat("a", 4097), wantErr: true},
		{name: "4096 multibyte characters", value: strings.Repeat("界", 4096)},
		{name: "4097 multibyte characters", value: strings.Repeat("界", 4097), wantErr: true},
		{name: "invalid UTF-8", value: string([]byte{0xff}), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, err := loadTestConfig(t)
			if err != nil {
				t.Fatal(err)
			}
			v.Set("telegram.startMessage", test.value)

			_, parseErr := parseConfig(v)
			if test.wantErr && parseErr == nil {
				t.Fatal("expected invalid start message to fail")
			}
			if !test.wantErr && parseErr != nil {
				t.Fatalf("expected valid start message, got %v", parseErr)
			}
		})
	}
}

func TestLoadConfigPreservesLiteralDollarSigns(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	configWithDollarSigns := strings.Replace(
		testConfig,
		"StartMessage: Welcome",
		"StartMessage: 'The price is $5 and $UNSET stays literal'",
		1,
	)
	v, err := loadTestConfigContent(t, configWithDollarSigns)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := parseConfig(v)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.StartMessage != "The price is $5 and $UNSET stays literal" {
		t.Fatalf("literal dollar signs changed: %q", cfg.Telegram.StartMessage)
	}
}

func TestParseConfigRejectsInvalidTelegramTimings(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	tests := []struct {
		name      string
		key       string
		value     string
		errorPart string
	}{
		{
			name:      "polling timeout lacks request margin",
			key:       "telegram.bot.polling.timeout",
			value:     "31s",
			errorPart: "RequestTimeout must be at least 36s",
		},
		{
			name:      "polling lease equals request timeout",
			key:       "telegram.bot.polling.leaseDuration",
			value:     "35s",
			errorPart: "Bot.Polling.LeaseDuration must exceed RequestTimeout",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, err := loadTestConfig(t)
			if err != nil {
				t.Fatal(err)
			}
			v.Set(test.key, test.value)

			_, parseErr := parseConfig(v)
			if parseErr == nil {
				t.Fatal("expected invalid Telegram timing configuration to fail")
			}
			if !strings.Contains(parseErr.Error(), test.errorPart) {
				t.Fatalf("unexpected validation error: %v", parseErr)
			}
		})
	}
}

func TestTrackedConfigDefaults(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://postgres@localhost:5432/tg_bot_proxy?sslmode=disable")
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("TELEGRAM_BOT_ID", "1")
	t.Setenv("TELEGRAM_SUPERGROUP_ID", "-1001")

	v, err := loadConfig("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parseConfig(v)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Postgres.DSN != "postgres://postgres@localhost:5432/tg_bot_proxy?sslmode=disable" {
		t.Fatalf("unexpected default PostgreSQL DSN: %s", cfg.Postgres.DSN)
	}
	if cfg.Telegram.RequestTimeout != 35*time.Second {
		t.Fatalf("unexpected default Telegram request timeout: %s", cfg.Telegram.RequestTimeout)
	}
	if cfg.Telegram.Bot.Polling.LeaseDuration != time.Minute {
		t.Fatalf("unexpected default polling lease: %s", cfg.Telegram.Bot.Polling.LeaseDuration)
	}
}

func loadTestConfig(t *testing.T) (*viper.Viper, error) {
	t.Helper()
	return loadTestConfigContent(t, testConfig)
}

func loadTestConfigContent(t *testing.T, content string) (*viper.Viper, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return loadConfig(path)
}

const testConfig = `
Namespace: test
Subsystem: telegram_proxy
Tempo:
  URL: localhost:4317
  ServiceName: telegram-proxy
  ServiceVersion: test
Postgres:
  DSN: postgres://localhost/test
  MaxConnections: 2
  MinConnections: 1
  MinIdleConnections: 1
  MaxConnLifetime: 1m
Telegram:
  BaseURL: https://api.telegram.org
  RequestTimeout: 35s
  Bot:
    ID: 1
    Token:
    Polling:
      Interval: 1s
      Timeout: 30s
      LeaseDuration: 1m
      BatchSize: 100
      AllowedUpdates: [message]
  SupergroupID: -1001
  StartMessage: Welcome
Inbox:
  Interval: 1s
  BatchSize: 100
  LeaseDuration: 30s
  Retry:
    MaxAttempts: 5
    InitialDelay: 5s
    MaxDelay: 1h
    Multiplier: 3
Outbox:
  Interval: 1s
  BatchSize: 20
  WorkerPoolSize: 10
  LeaseDuration: 1m
  Retry:
    MaxAttempts: 10
    InitialDelay: 5s
    MaxDelay: 1h
    Multiplier: 2
Service:
  GRPCPort: 5050
  HTTPPort: 8080
  CORS:
    AllowedOrigins: [http://localhost]
    AllowedMethods: [GET]
    AllowedHeaders: [Content-Type]
MonitoringService:
  HTTPPort: 9090
  ReadHeaderTimeout: 10s
`
