package sync

import (
	"context"
	"log"
	"time"

	"github.com/klzn-labs/kalzone/pkg/events"
	"go.uber.org/fx"
)

const (
	resyncEvery = 5 * time.Minute
	syncTimeout = 2 * time.Minute
)

type Syncer interface {
	SyncsOn(action string) bool
	Sync(ctx context.Context) error
}

type Worker struct {
	syncer Syncer
	every  time.Duration
	dirty  chan struct{}
	stop   chan struct{}
	done   chan struct{}
}

func NewWorker(syncer Syncer, every time.Duration) *Worker {
	return &Worker{syncer: syncer, every: every, dirty: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
}

func NewDefaultWorker(syncer Syncer) *Worker {
	return NewWorker(syncer, resyncEvery)
}

func OnStart(lc fx.Lifecycle, w *Worker) {
	lc.Append(fx.StartStopHook(w.Start, w.Stop))
}

func (w *Worker) Handle(_ context.Context, e events.Event) error {
	if w.syncer.SyncsOn(e.Action) {
		w.mark()
	}
	return nil
}

func (w *Worker) Start() {
	w.mark()
	go w.run()
}

func (w *Worker) Stop() {
	close(w.stop)
	<-w.done
}

func (w *Worker) mark() {
	select {
	case w.dirty <- struct{}{}:
	default:
	}
}

func (w *Worker) run() {
	defer close(w.done)
	tick := time.NewTicker(w.every)
	defer tick.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-w.dirty:
			w.sync()
		case <-tick.C:
			w.sync()
		}
	}
}

func (w *Worker) sync() {
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	if err := w.syncer.Sync(ctx); err != nil {
		log.Printf("dashboards: %v", err)
	}
}
