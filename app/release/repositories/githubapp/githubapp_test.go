package githubapp_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/app/release/repositories/githubapp"
	"github.com/adrianozp/platform/internal/fakegithub"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/stretchr/testify/require"
)

func privateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func Test_Dispatch_UsesACachedInstallationToken(t *testing.T) {
	server := fakegithub.New()
	defer server.Close()
	client, err := githubapp.NewClient(config.Config{GitHub: config.GitHub{AppID: 1, AppPrivateKey: privateKeyPEM(t), APIURL: server.URL}})
	require.NoError(t, err)
	request := entities.BuildRequest{Repository: "o/api", Workflow: "platform-build.yml", Ref: "main", Inputs: map[string]string{"release_id": "r1"}}

	require.NoError(t, client.Dispatch(context.Background(), request))
	require.NoError(t, client.Dispatch(context.Background(), request))

	require.Len(t, server.Dispatches(), 2)
	require.Equal(t, fakegithub.Dispatch{Repository: "o/api", Workflow: "platform-build.yml", Ref: "main", Inputs: map[string]string{"release_id": "r1"}}, server.Dispatches()[0])
	require.Equal(t, 1, server.TokensIssued())
}

func Test_Dispatch_WithoutAnAppIsNotConfigured(t *testing.T) {
	client, err := githubapp.NewClient(config.Config{})
	require.NoError(t, err)

	err = client.Dispatch(context.Background(), entities.BuildRequest{Repository: "o/api"})

	require.ErrorIs(t, err, apperror.New(apperror.CodeNotConfigured, ""))
}

func Test_NewClient_RejectsABrokenKey(t *testing.T) {
	_, err := githubapp.NewClient(config.Config{GitHub: config.GitHub{AppID: 1, AppPrivateKey: "not a key"}})

	require.Error(t, err)
}
