package eventbus

import (
	"context"
	"log"

	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/events"
	"go.uber.org/fx"
)

type Params struct {
	fx.In
	Subscribers []events.Subscriber `group:"subscribers"`
}

type Bus struct {
	subscribers []events.Subscriber
}

func New(p Params) *Bus {
	return &Bus{subscribers: p.Subscribers}
}

func (b *Bus) Subscribe(s events.Subscriber) {
	b.subscribers = append(b.subscribers, s)
}

func (b *Bus) Publish(ctx context.Context, event events.Event) {
	if event.At.IsZero() {
		event.At = clock.Now().UTC()
	}
	for _, s := range b.subscribers {
		if err := s.Handle(ctx, event); err != nil {
			log.Printf("event %s on %s %s was not handled: %v", event.Action, event.TargetKind, event.TargetID, err)
		}
	}
}
