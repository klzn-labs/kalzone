package eventbus_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adrianozp/platform/internal/eventbus"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/stretchr/testify/require"
)

type recorder struct {
	got []events.Event
	err error
}

func (r *recorder) Handle(_ context.Context, e events.Event) error {
	r.got = append(r.got, e)
	return r.err
}

func Test_PublishReachesEverySubscriberEvenAfterAFailure(t *testing.T) {
	failing := &recorder{err: errors.New("down")}
	healthy := &recorder{}
	bus := eventbus.New(eventbus.Params{Subscribers: []events.Subscriber{failing, healthy}})

	bus.Publish(context.Background(), events.Event{Action: "service.created"})

	require.Len(t, failing.got, 1)
	require.Len(t, healthy.got, 1)
	require.False(t, healthy.got[0].At.IsZero())
}
