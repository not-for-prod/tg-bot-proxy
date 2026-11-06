package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"tg-bot-proxy/internal/pkg/connector/postgres"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type Config struct {
	Namespace string `validate:"required"`
	Subsystem string `validate:"required"`
	Tempo     struct {
		URL            string `validate:"required"`
		ServiceName    string `validate:"required"`
		ServiceVersion string `validate:"required"`
	}
	Postgres          postgres.Config `validate:"required"`
	Telegram          Telegram        `validate:"required"`
	Inbox             Inbox           `validate:"required"`
	Outbox            Outbox          `validate:"required"`
	Service           ClayServer      `validate:"required"`
	MonitoringService struct {
		HTTPPort          int           `validate:"required"`
		ReadHeaderTimeout time.Duration `validate:"required"`
	}
}

type Telegram struct {
	BaseURL        string        `validate:"required,https_url"`
	RequestTimeout time.Duration `validate:"required,min=1"`
	Bot            Bot           `validate:"required"`
	SupergroupID   int64         `validate:"required"`
	StartMessage   string        `validate:"required,max=4096"`
}

type Bot struct {
	ID      int64   `validate:"required"`
	Token   string  `validate:"required"`
	Polling Polling `validate:"required"`
}

type Polling struct {
	Interval       time.Duration `validate:"required,min=1"`
	Timeout        time.Duration `validate:"required,min=1"`
	LeaseDuration  time.Duration `validate:"required,min=1"`
	BatchSize      int           `validate:"required,min=1,max=100"`
	AllowedUpdates []string      `validate:"required,min=1,dive,required"`
}

type Retry struct {
	MaxAttempts  int           `validate:"required,min=1"`
	InitialDelay time.Duration `validate:"required,min=1"`
	MaxDelay     time.Duration `validate:"required,min=1"`
	Multiplier   float64       `validate:"required,gte=1"`
}

type Inbox struct {
	Interval      time.Duration `validate:"required,min=1"`
	BatchSize     int           `validate:"required,min=1,max=1000"`
	LeaseDuration time.Duration `validate:"required,min=1"`
	Retry         Retry         `validate:"required"`
}

type Outbox struct {
	Interval       time.Duration `validate:"required,min=1"`
	BatchSize      int           `validate:"required,min=1"`
	WorkerPoolSize int           `validate:"required,min=1,max=100"`
	LeaseDuration  time.Duration `validate:"required,min=1000000000"`
	Retry          Retry         `validate:"required"`
}

type ClayServer struct {
	GRPCPort int `validate:"required"`
	HTTPPort int `validate:"required"`
	CORS     struct {
		AllowedOrigins   []string
		AllowedMethods   []string
		AllowedHeaders   []string
		ExposedHeaders   []string
		AllowCredentials bool
		MaxAge           int
	} `validate:"required"`
}

type Worker struct {
	Disable   bool
	Interval  time.Duration `validate:"required,min=1"`
	BatchSize uint64        `validate:"required,min=1"`
}

const minimumRequestTimeoutMargin = 5 * time.Second

//nolint:gochecknoglobals // package-level singleton cache guarded by sync.Once
var (
	once   sync.Once
	config *Config
)

func Instance() *Config {
	if config == nil {
		once.Do(
			func() {
				var err error

				path := "./config/config.yaml"

				root, err := getGoModRoot()
				if err == nil {
					path = root + "/config/config.yaml"
				}

				viperCfg, err := loadConfig(path)
				if err != nil {
					panic("error loading config file: " + err.Error())
				}

				cfg, err := parseConfig(viperCfg)
				if err != nil {
					panic("error parsing config file: " + err.Error())
				}

				config = cfg
			},
		)
	}

	return config
}

// getGoModRoot returns the absolute path to the root directory containing go.mod.
func getGoModRoot() (string, error) {
	output, err := exec.CommandContext(context.Background(), "go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func loadConfig(path string) (*viper.Viper, error) {
	v := viper.New()
	if err := v.BindEnv("Postgres.DSN", "POSTGRES_DSN"); err != nil {
		return nil, fmt.Errorf("bind postgres DSN: %w", err)
	}
	if err := v.BindEnv("Telegram.Bot.Token", "TELEGRAM_BOT_TOKEN"); err != nil {
		return nil, fmt.Errorf("bind Telegram bot token: %w", err)
	}
	if err := v.BindEnv("Telegram.Bot.ID", "TELEGRAM_BOT_ID"); err != nil {
		return nil, fmt.Errorf("bind Telegram bot ID: %w", err)
	}
	if err := v.BindEnv("Telegram.SupergroupID", "TELEGRAM_SUPERGROUP_ID"); err != nil {
		return nil, fmt.Errorf("bind Telegram supergroup ID: %w", err)
	}

	configRaw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't load config file %s: %w", path, err)
	}

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	if err = v.ReadConfig(strings.NewReader(string(configRaw))); err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	return v, nil
}

func parseConfig(v *viper.Viper) (*Config, error) {
	var c Config

	err := v.Unmarshal(&c)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %w", err)
	}

	err = validator.New().Struct(&c)
	if err != nil {
		return nil, fmt.Errorf("can't validate config: %s", err.Error())
	}
	if !utf8.ValidString(c.Telegram.StartMessage) {
		return nil, errors.New("can't validate config: Telegram StartMessage must be valid UTF-8")
	}
	if strings.TrimSpace(c.Telegram.StartMessage) == "" {
		return nil, errors.New("can't validate config: Telegram StartMessage must not be blank")
	}
	minimumRequestTimeout := c.Telegram.Bot.Polling.Timeout + minimumRequestTimeoutMargin
	if c.Telegram.RequestTimeout < minimumRequestTimeout {
		return nil, fmt.Errorf(
			"can't validate config: Telegram RequestTimeout must be at least %s "+
				"(Bot.Polling.Timeout %s plus %s margin)",
			minimumRequestTimeout,
			c.Telegram.Bot.Polling.Timeout,
			minimumRequestTimeoutMargin,
		)
	}
	if c.Telegram.Bot.Polling.LeaseDuration <= c.Telegram.RequestTimeout {
		return nil, fmt.Errorf(
			"can't validate config: Telegram Bot.Polling.LeaseDuration must exceed RequestTimeout %s",
			c.Telegram.RequestTimeout,
		)
	}

	return &c, nil
}
