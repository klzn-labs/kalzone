package oidc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/fakeoidc"
	"github.com/klzn-labs/kalzone/internal/oidc"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

const audience = "https://platform.test"

func setup(t *testing.T) (*fakeoidc.Issuer, *oidc.Verifier) {
	t.Helper()
	issuer := fakeoidc.New()
	t.Cleanup(issuer.Close)
	return issuer, oidc.NewVerifier(config.Config{OIDC: config.OIDC{Issuer: issuer.URL, Audience: audience}})
}

func Test_Verify_AcceptsAValidToken(t *testing.T) {
	issuer, verifier := setup(t)

	claims, err := verifier.Verify(context.Background(), issuer.Mint(fakeoidc.Token{Audience: audience, Repository: "o/api", Expires: time.Minute}))

	require.NoError(t, err)
	require.Equal(t, "o/api", claims.Repository)
}

func Test_Verify_RefusesBadTokens(t *testing.T) {
	issuer, verifier := setup(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	bad := map[string]string{
		"audience":  issuer.Mint(fakeoidc.Token{Audience: "https://elsewhere", Repository: "o/api", Expires: time.Minute}),
		"expired":   issuer.Mint(fakeoidc.Token{Audience: audience, Repository: "o/api", Expires: -time.Hour}),
		"issuer":    issuer.Mint(fakeoidc.Token{Audience: audience, Repository: "o/api", Expires: time.Minute, Issuer: "https://evil"}),
		"signature": issuer.Mint(fakeoidc.Token{Audience: audience, Repository: "o/api", Expires: time.Minute, Key: other}),
		"garbage":   "not.a.token",
	}
	for name, token := range bad {
		_, err := verifier.Verify(context.Background(), token)
		require.Error(t, err, name)
	}
}
