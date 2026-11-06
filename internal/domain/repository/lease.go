package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrStaleLease = errors.New("stale lease")

type Lease struct {
	BotID   int64
	Owner   uuid.UUID
	Version int64
	Offset  int64
}

type ClaimLeaseRequest struct {
	BotID      int64
	Owner      uuid.UUID
	LeaseUntil time.Time
}
