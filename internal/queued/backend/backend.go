package backend

import (
	"context"
	"errors"
	"time"

	"github.com/adrianozp/platform/internal/queued/model"
)

var ErrDeadNotFound = errors.New("dead letter not found")

type Backend interface {
	Ensure(ctx context.Context, q model.Queue) error
	Remove(ctx context.Context, q model.Queue) error
	Publish(ctx context.Context, q model.Queue, m model.Outgoing) (string, error)
	Fetch(ctx context.Context, q model.Queue, max int, wait time.Duration) ([]model.Delivery, error)
	MoveDue(ctx context.Context, q model.Queue, max int) (int, error)
	Ack(ctx context.Context, d model.Delivery) error
	Retry(ctx context.Context, d model.Delivery, after time.Duration) error
	Bury(ctx context.Context, q model.Queue, d model.Delivery, reason model.DeadReason) error
	Dead(ctx context.Context, q model.Queue, limit int) ([]model.DeadLetter, error)
	Replay(ctx context.Context, q model.Queue, id string) error
	ReplayAll(ctx context.Context, q model.Queue) (int, error)
	PurgeDead(ctx context.Context, q model.Queue) error
	Stats(ctx context.Context, q model.Queue) (model.BackendStats, error)
}
