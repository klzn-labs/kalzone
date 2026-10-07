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

func Start(lc fx.Lifecycle, name string, sweeper Sweeper) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go loop(ctx, name, sweeper)
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
}

func loop(ctx context.Context, name string, sweeper Sweeper) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepOnce(ctx, name, sweeper)
		}
	}
}

func sweepOnce(ctx context.Context, name string, sweeper Sweeper) {
	swept, err := sweeper.Sweep(ctx)
	if err != nil {
		log.Printf("%s sweeper: %v", name, err)
		return
	}
	if swept > 0 {
		log.Printf("%s sweeper: timed out %d", name, swept)
	}
}
