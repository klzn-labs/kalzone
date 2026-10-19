package queues

import (
	"context"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const backingUpAfter = 5 * time.Minute

type Row struct {
	Queue      entities.Queue
	Consumer   string
	Publishers string
	Attached   []string
	Stats      entities.Stats
	Fresh      bool
	State      string
	Badge      string
}

type detailView struct {
	Row      Row
	Tab      string
	Edit     bool
	Versions []entities.Version
}

func tab(raw string) string {
	switch raw {
	case "settings", "activity":
		return raw
	}
	return "dead"
}

func (h QueuesHandler) row(ctx context.Context, q entities.Queue) Row {
	st, _, _ := h.queues.Stats(ctx, q.Environment, q.Name)
	consumer, _ := h.names.SlugOf(ctx, q.ConsumerID)
	r := Row{Queue: q, Consumer: consumer, Publishers: strings.Join(h.slugs(ctx, q.Publishers), ", "), Attached: h.slugs(ctx, q.Attached()), Stats: st, Fresh: st.Fresh(clock.Now().UTC())}
	r.State, r.Badge = state(r)
	return r
}

func (h QueuesHandler) slugs(ctx context.Context, ids []string) []string {
	names := []string{}
	for _, id := range ids {
		if slug, err := h.names.SlugOf(ctx, id); err == nil {
			names = append(names, slug)
		}
	}
	return names
}

func state(r Row) (string, string) {
	switch {
	case r.Queue.Paused:
		return "Paused", "badge-danger"
	case !r.Fresh:
		return "No data", ""
	case r.Stats.OldestAgeSeconds > backingUpAfter.Seconds() || r.Stats.DeadLetters > 0:
		return "Backing up", "badge-warn"
	}
	return "Healthy", "badge-ok"
}
