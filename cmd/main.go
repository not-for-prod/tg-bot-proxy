package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"tg-bot-proxy/config"
	chatservice "tg-bot-proxy/internal/application/service/chat"
	chatworker "tg-bot-proxy/internal/delivery/worker/chat"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/domain/service"
	telegramgateway "tg-bot-proxy/internal/infrastructure/gateway/telegram"
	chatrepository "tg-bot-proxy/internal/infrastructure/repository/chat"
	inboxrepository "tg-bot-proxy/internal/infrastructure/repository/inbox"
	outboxrepository "tg-bot-proxy/internal/infrastructure/repository/outbox"
	"tg-bot-proxy/internal/pkg/connector/postgres"
	"tg-bot-proxy/internal/pkg/healthcheck"
	grpcmw "tg-bot-proxy/internal/pkg/mw/grpc"
	httpmw "tg-bot-proxy/internal/pkg/mw/http"
	"tg-bot-proxy/internal/pkg/worker"

	"buf.build/go/protovalidate"
	txManagerPgxv5 "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-co-op/gocron/v2"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/not-for-prod/clay/server"
	"github.com/not-for-prod/clay/server/middlewares/mwgrpc"
	"github.com/not-for-prod/clay/server/middlewares/mwhttp"
	"github.com/not-for-prod/observer/logger"
	"github.com/not-for-prod/observer/logger/zap"
	"github.com/not-for-prod/observer/tracer"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
)

type fxLogger struct{}

func newFxLogger() *fxLogger {
	return &fxLogger{}
}

func (l *fxLogger) Printf(msg string, args ...any) {
	logger.Instance().Info(msg, args)
}

func main() {
	logger.SetLogger(zap.NewLogger())
	fx.New(
		fx.Logger(newFxLogger()),
		fx.Provide(
			prometheus.NewRegistry,
			protovalidate.New,
			provideMetrics,
			provideHealthcheck,
			providePostgres,
			provideAvitoTxManager,
			// ===== infrastructure =====
			// repository
			fx.Annotate(inboxrepository.NewImplementation, fx.As(new(repository.Inbox))),
			fx.Annotate(chatrepository.NewImplementation, fx.As(new(repository.Chat))),
			fx.Annotate(outboxrepository.NewImplementation, fx.As(new(repository.Outbox))),
			// gateway
			fx.Annotate(telegramgateway.NewImplementation, fx.As(new(gateway.Telegram))),
			// ===== application =====
			// commands
			provideTelegramProxyService,
			// queries
			// ===== delivery =====
			// workers
			provideWorkers,
			// services
		),
		fx.Invoke(
			config.Instance,
			invokeLogger,
			invokeTraces,
			invokeWorkers,
			invokeService,
			invokeMonitoringService,
		),
	).Run()
}

type telegramProxyServiceParams struct {
	fx.In

	Inbox    repository.Inbox
	Chats    repository.Chat
	Outbox   repository.Outbox
	Telegram gateway.Telegram
	Tx       trm.Manager
	Metrics  *chatservice.Metrics
}

func provideTelegramProxyService(params telegramProxyServiceParams) service.TelegramProxy {
	return chatservice.NewImplementation(chatservice.Dependencies{
		Inbox:    params.Inbox,
		Chats:    params.Chats,
		Outbox:   params.Outbox,
		Telegram: params.Telegram,
		Tx:       params.Tx,
		Metrics:  params.Metrics,
	})
}

func providePostgres() (*pgxpool.Pool, error) {
	return postgres.New(config.Instance().Postgres)
}

func provideAvitoTxManager(pg *pgxpool.Pool) (trm.Manager, *txManagerPgxv5.CtxGetter) {
	return manager.Must(txManagerPgxv5.NewDefaultFactory(pg)), txManagerPgxv5.DefaultCtxGetter
}

type metricsOut struct {
	fx.Out

	HTTP          *mwhttp.ServerMetrics
	GRPC          *grpcprom.ServerMetrics
	Worker        *worker.Metrics
	TelegramProxy *chatservice.Metrics
}

func provideMetrics(
	registry *prometheus.Registry,
) metricsOut {
	httpMetrics := mwhttp.NewServerMetrics(
		mwhttp.WithNamespace(config.Instance().Namespace),
		mwhttp.WithSubsystem(config.Instance().Subsystem),
	)
	grpcMetrics := mwgrpc.NewServerMetrics(
		mwgrpc.WithNamespace(config.Instance().Namespace),
		mwgrpc.WithSubsystem(config.Instance().Subsystem),
	)

	registry.MustRegister(httpMetrics)
	registry.MustRegister(grpcMetrics)
	workerMetrics := worker.NewMetrics(registry, config.Instance().Namespace, config.Instance().Subsystem)
	proxyMetrics := chatservice.NewMetrics(
		registry,
		config.Instance().Namespace,
		config.Instance().Subsystem,
	)

	return metricsOut{
		HTTP:          httpMetrics,
		GRPC:          grpcMetrics,
		Worker:        workerMetrics,
		TelegramProxy: proxyMetrics,
	}
}

func provideWorkers(proxy service.TelegramProxy, metrics *worker.Metrics) []worker.Worker {
	return []worker.Worker{
		chatworker.NewPoller(proxy, metrics),
		chatworker.NewInboxProcessor(proxy, metrics),
		chatworker.NewOutboxPublisher(proxy, metrics),
	}
}

func provideHealthcheck(pg *pgxpool.Pool) healthcheck.Handler {
	handler := healthcheck.NewHandler()
	handler.AddReadinessCheck("postgres", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		return pg.Ping(ctx)
	})

	return handler
}

func invokeLogger(lc fx.Lifecycle) {
	lc.Append(
		fx.Hook{
			OnStart: func(_ context.Context) error {
				return nil
			},
			OnStop: logger.Stop,
		},
	)
}

func invokeTraces(lc fx.Lifecycle) {
	tp := tracer.NewProvider(
		tracer.WithHost(config.Instance().Tempo.URL),
		tracer.WithServiceName(config.Instance().Tempo.ServiceName),
		tracer.WithServiceVersion(config.Instance().Tempo.ServiceVersion),
	)
	lc.Append(
		fx.Hook{
			OnStart: func(context.Context) error {
				return tp.Start(context.Background())
			},
			OnStop: func(ctx context.Context) error {
				return tp.Stop(ctx)
			},
		},
	)
}

func invokeWorkers(
	lc fx.Lifecycle,
	workers []worker.Worker,
) error {
	s, err := gocron.NewScheduler()
	if err != nil {
		return err
	}

	for _, w := range workers {
		if w.Disabled() {
			continue
		}

		_, err = s.NewJob(
			gocron.DurationJob(w.Interval()),
			gocron.NewTask(
				w.Job,
			),
			gocron.WithSingletonMode(gocron.LimitModeReschedule),
		)
		if err != nil {
			return err
		}
	}

	lc.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				s.Start()
				return nil
			},
			OnStop: func(ctx context.Context) error {
				return s.ShutdownWithContext(ctx)
			},
		},
	)

	return nil
}

func invokeService(
	lc fx.Lifecycle,
	shutdowner fx.Shutdowner,
	validator protovalidate.Validator,
	grpcMetrics *grpcprom.ServerMetrics,
	httpMetrics *mwhttp.ServerMetrics,
	// TODO: << delivery services >>
) {
	corsCfg := config.Instance().Service.CORS
	corsHandler := cors.Handler(
		cors.Options{
			// AllowedOrigins:   []string{"https://foo.com"}, // Use this to allow specific origin hosts
			AllowedOrigins: corsCfg.AllowedOrigins,
			// AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
			AllowedMethods:   corsCfg.AllowedMethods,
			AllowedHeaders:   corsCfg.AllowedHeaders,
			ExposedHeaders:   corsCfg.ExposedHeaders,
			AllowCredentials: corsCfg.AllowCredentials,
			MaxAge:           corsCfg.MaxAge, // Maximum value not ignored by any of major browsers
		},
	)

	serviceServer := server.NewServer(
		config.Instance().Service.GRPCPort,
		server.WithHTTPPort(config.Instance().Service.HTTPPort),
		server.WithGRPCUnaryMiddlewares(
			grpcMetrics.UnaryServerInterceptor(),
			proterrors.UnaryServerInterceptor(),
			grpcmw.ProtoValidate(validator),
		),
		server.WithHTTPMiddlewares(
			httpMetrics.Middleware(),
			corsHandler,
			middleware.RealIP,
			httpmw.TraceIDHeader,
		),
	)

	lc.Append(
		fx.Hook{
			OnStart: func(_ context.Context) error {
				go func() {
					// TODO: fill this with << delivery services >>
					// ex. examplev1.NewExampleServiceServiceDesc(<< delivery service >>),
					err := serviceServer.Run()
					if err != nil && !errors.Is(err, http.ErrServerClosed) {
						logger.Instance().Error(err.Error())
						_ = shutdowner.Shutdown(fx.ExitCode(1))
					}
				}()

				return nil
			},
			OnStop: func(ctx context.Context) error {
				return serviceServer.Stop(ctx)
			},
		},
	)
}

func invokeMonitoringService(
	lc fx.Lifecycle,
	shutdowner fx.Shutdowner,
	healthcheckHandler healthcheck.Handler,
	registry *prometheus.Registry,
) error {
	mux := chi.NewMux()
	mux.Handle("/healthcheck/*", healthcheckHandler)
	mux.Mount("/debug", middleware.Profiler())
	mux.Handle(
		"/metrics",
		promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics: true}),
	)

	httpServer := http.Server{
		Handler:           mux,
		ReadHeaderTimeout: config.Instance().MonitoringService.ReadHeaderTimeout,
	}

	httpListener, err := (&net.ListenConfig{}).Listen(
		context.Background(),
		"tcp",
		net.JoinHostPort("", strconv.Itoa(config.Instance().MonitoringService.HTTPPort)),
	)
	if err != nil {
		return err
	}

	lc.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				go func() {
					err = httpServer.Serve(httpListener)
					if err != nil {
						logger.Instance().Error(err.Error())
						_ = shutdowner.Shutdown(fx.ExitCode(1))
					}
				}()

				return nil
			},
			OnStop: httpServer.Shutdown,
		},
	)

	return nil
}
