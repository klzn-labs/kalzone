package github_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/repositories/github"
	"github.com/klzn-labs/kalzone/internal/fakegithub"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

func newClient(server *fakegithub.Server) github.Client {
	return github.NewClient(config.Config{GitHub: config.GitHub{
		ClientID: "id", ClientSecret: "secret", OAuthURL: server.URL, APIURL: server.URL,
	}})
}

func Test_ExchangeThenCurrentUser(t *testing.T) {
	server := fakegithub.New()
	defer server.Close()
	server.WillSignIn("good", fakegithub.User{ID: 42, Login: "ana", Name: "Ana", AvatarURL: "a.png"})
	client := newClient(server)

	token, err := client.Exchange(context.Background(), "good", "http://x/cb")
	require.NoError(t, err)
	user, err := client.CurrentUser(context.Background(), token)

	require.NoError(t, err)
	require.Equal(t, entities.GitHubUser{ID: 42, Login: "ana", Name: "Ana", AvatarURL: "a.png"}, user)
}

func Test_Exchange_BadCodeIsAnError(t *testing.T) {
	server := fakegithub.New()
	defer server.Close()

	_, err := newClient(server).Exchange(context.Background(), "bad", "")

	require.ErrorIs(t, err, github.ErrNoAccessToken)
}

func Test_AuthorizeURL(t *testing.T) {
	client := github.NewClient(config.Config{GitHub: config.GitHub{ClientID: "id", OAuthURL: "https://github.com"}})

	parsed, err := url.Parse(client.AuthorizeURL("st", "https://p/cb"))

	require.NoError(t, err)
	require.Equal(t, "/login/oauth/authorize", parsed.Path)
	require.Equal(t, "st", parsed.Query().Get("state"))
	require.Equal(t, "https://p/cb", parsed.Query().Get("redirect_uri"))
}
