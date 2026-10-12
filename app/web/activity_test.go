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

func Test_ActivityFrom_KeepsTheTokenAndPhrasesBypass(t *testing.T) {
	events := []audit.Event{
		{ActorLogin: "bob", TokenID: "t1", TokenName: "claude-local", Action: "config.revised", TargetName: "api"},
		{ActorLogin: "bob", Action: "token.bypass_used", Environment: "prod", Details: map[string]any{"action": "deploy"}},
	}

	got := web.ActivityFrom(events)

	require.Equal(t, "claude-local", got[0].TokenName)
	require.Equal(t, "t1", got[0].TokenID)
	require.Equal(t, "deploy on prod", got[1].Detail)
}

func Test_ActivityFrom_LinksRequestEventsToTheRequest(t *testing.T) {
	events := []audit.Event{
		{ActorLogin: "mia", Action: "request.approved", TargetID: "c1", TargetName: "api", Details: map[string]any{"summary": "deploy release #2 to prod"}},
	}

	got := web.ActivityFrom(events)

	require.Equal(t, "approved a request on", got[0].Phrase)
	require.Equal(t, "/requests/c1", got[0].Link)
	require.Equal(t, "deploy release #2 to prod", got[0].Detail)
}
