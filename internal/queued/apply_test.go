package queued

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

type seenQueues struct{ commands []queuedapi.Command }

func (s *seenQueues) Apply(_ context.Context, d queuedapi.Desired) ([]string, []error) {
	s.commands = d.Commands
	return []string{"q-done"}, nil
}

type seenCrons struct {
	crons    []queuedapi.Cron
	commands []queuedapi.Command
}

func (s *seenCrons) Apply(crons []queuedapi.Cron, commands []queuedapi.Command) ([]string, []error) {
	s.crons, s.commands = crons, commands
	return []string{"c-done"}, nil
}

func Test_Applier_RoutesCronCommandsAwayFromTheQueues(t *testing.T) {
	queues, crons := &seenQueues{}, &seenCrons{}
	desired := queuedapi.Desired{
		Crons:    []queuedapi.Cron{{ID: "cron1"}},
		Commands: []queuedapi.Command{{ID: "k1", QueueID: "q1", Kind: "replay"}, {ID: "k2", CronID: "cron1", Kind: "run"}},
	}

	done, errs := applier{queues: queues, crons: crons}.Apply(context.Background(), desired)

	require.Empty(t, errs)
	require.Equal(t, []string{"q-done", "c-done"}, done)
	require.Equal(t, []queuedapi.Command{desired.Commands[0]}, queues.commands)
	require.Equal(t, []queuedapi.Command{desired.Commands[1]}, crons.commands)
	require.Equal(t, desired.Crons, crons.crons)
}
