package entities_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/stretchr/testify/require"
)

var created = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func lines(messages ...string) []entities.Event {
	result := []entities.Event{}
	for i, m := range messages {
		result = append(result, entities.Event{ID: int64(i + 1), Message: m})
	}
	return result
}

func Test_Summarize_SkipsDockerProgressAndKeepsTheLastTwentyLines(t *testing.T) {
	messages := []string{"5f70bf18a086 Pulling fs layer", " ✔ 5f70bf18a086 Pull complete", "a1b2c3d4e5f6: Downloading [==>  ] 1MB/3MB"}
	for i := 1; i <= 25; i++ {
		messages = append(messages, fmt.Sprintf("step %d", i))
	}
	messages = append(messages, "deadbeefcafe0 is not a layer id")

	s := entities.Summarize(entities.Deployment{CreatedAt: created}, lines(messages...), 400, created)

	require.Equal(t, 400, s.Total)
	require.Equal(t, 3, s.ProgressLines)
	require.Len(t, s.Tail, 20)
	require.Equal(t, "step 7", s.Tail[0].Message)
	require.Equal(t, "deadbeefcafe0 is not a layer id", s.Tail[19].Message)
}

func Test_Phases_FollowTheDeploymentsTimestamps(t *testing.T) {
	started, finished := created.Add(4*time.Second), created.Add(75*time.Second)
	now := created.Add(90 * time.Second)

	pending := entities.Deployment{CreatedAt: created}.Phases(now)
	running := entities.Deployment{CreatedAt: created, StartedAt: &started}.Phases(now)
	done := entities.Deployment{CreatedAt: created, StartedAt: &started, FinishedAt: &finished}.Phases(now)
	cancelled := entities.Deployment{CreatedAt: created, FinishedAt: &finished}.Phases(now)

	require.Equal(t, []entities.Phase{{Name: "waiting", Seconds: 90}}, pending)
	require.Equal(t, []entities.Phase{{Name: "waiting", Seconds: 4}, {Name: "running", Seconds: 86}}, running)
	require.Equal(t, []entities.Phase{{Name: "waiting", Seconds: 4}, {Name: "running", Seconds: 71}}, done)
	require.Equal(t, []entities.Phase{{Name: "waiting", Seconds: 75}}, cancelled)
}

func Test_Summarize_WithoutLinesHasAnEmptyTail(t *testing.T) {
	s := entities.Summarize(entities.Deployment{CreatedAt: created}, nil, 0, created)

	require.NotNil(t, s.Tail)
	require.Empty(t, s.Tail)
}
