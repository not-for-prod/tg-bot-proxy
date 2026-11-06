package worker

import (
	"context"
	"time"

	"github.com/not-for-prod/observer/tracer/prospan"
)

type Worker interface {
	Disabled() bool
	Job(context.Context)
	Name() string
	Interval() time.Duration
}

var _ Worker = &Implementation{}

type Implementation struct {
	cfg     Config
	job     func(ctx context.Context) error
	metrics *Metrics
}

func New(cfg Config, job func(ctx context.Context) error, metrics *Metrics) *Implementation {
	return &Implementation{
		cfg:     cfg,
		job:     job,
		metrics: metrics,
	}
}

func (i Implementation) Job(ctx context.Context) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	startedAt := time.Now()
	if i.metrics != nil {
		i.metrics.active.WithLabelValues(i.Name()).Inc()
	}

	err := i.job(ctx)
	if i.metrics != nil {
		status := workerStatusSuccess
		if err != nil {
			status = workerStatusError
		}
		i.metrics.active.WithLabelValues(i.Name()).Dec()
		i.metrics.executions.WithLabelValues(i.Name(), status).Inc()
		i.metrics.duration.WithLabelValues(i.Name()).Observe(time.Since(startedAt).Seconds())
	}
	if err != nil {
		span.Logger().Error("failed to execute job", "name", i.Name(), "error", err)
	}
}

func (i Implementation) Name() string {
	return i.cfg.Name
}

func (i Implementation) Interval() time.Duration {
	return i.cfg.Interval
}

func (i Implementation) Disabled() bool {
	return i.cfg.Disable
}
