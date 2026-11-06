package worker

import "time"

type Config struct {
	Disable  bool
	Name     string        `validate:"required"`
	Interval time.Duration `validate:"required,min=1"`
}
