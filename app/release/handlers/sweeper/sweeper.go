package sweeper

import (
	"context"
	"log"
	"time"

	"go.uber.org/fx"
)

const interval = time.Minute

type Sweeper interface {
	Sweep(ctx context.Context) (int, error)
}

func Run(lc fx.Lifecycle, sweeper Sweeper) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go loop(ctx, sweeper)
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
}

func loop(ctx context.Context, sweeper Sweeper) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepOnce(ctx, sweeper)
		}
	}
}

func sweepOnce(ctx context.Context, sweeper Sweeper) {
	swept, err := sweeper.Sweep(ctx)
	if err != nil {
		log.Printf("release sweeper: %v", err)
		return
	}
	if swept > 0 {
		log.Printf("release sweeper: timed out %d builds", swept)
	}
}
