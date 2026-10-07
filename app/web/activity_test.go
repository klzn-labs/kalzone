package web_test

import (
	"testing"

	audit "github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/stretchr/testify/require"
)

func Test_ActivityFrom_PhrasesKnownEvents(t *testing.T) {
	events := []audit.Event{
		{ActorLogin: "bob", Action: "config.revised", TargetName: "api", Environment: "prod", Details: map[string]any{"revision": float64(3)}},
		{ActorLogin: "mia", Action: "team.member_put", TargetName: "places", Details: map[string]any{"login": "bob", "role": "member"}},
		{ActorLogin: "x", Action: "something.new", TargetName: "y"},
	}

	got := web.ActivityFrom(events)

	require.Equal(t, "changed the config of", got[0].Phrase)
	require.Equal(t, "/services/api/env/prod", got[0].Link)
	require.Equal(t, "revision 3", got[0].Detail)
	require.Equal(t, "@bob → member", got[1].Detail)
	require.Equal(t, "something.new", got[2].Phrase)
}
