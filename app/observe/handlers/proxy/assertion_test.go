package proxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

func encodedKey(t *testing.T) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func signer(t *testing.T) Signer {
	t.Helper()
	s, err := NewSigner(config.Config{Grafana: config.Grafana{JWTKey: encodedKey(t)}})
	require.NoError(t, err)
	return s
}

func publicKey(t *testing.T, s Signer) ed25519.PublicKey {
	t.Helper()
	key := s.KeySet()["keys"].([]map[string]string)[0]
	raw, err := base64.RawURLEncoding.DecodeString(key["x"])
	require.NoError(t, err)
	return ed25519.PublicKey(raw)
}

func Test_Signer_IsOffWithoutAKeyAndRefusesAnotherKind(t *testing.T) {
	off, err := NewSigner(config.Config{})
	_, bad := NewSigner(config.Config{Grafana: config.Grafana{JWTKey: base64.StdEncoding.EncodeToString([]byte("not a key"))}})

	require.NoError(t, err)
	require.False(t, off.Enabled())
	require.ErrorIs(t, bad, errNotEd25519)
}

func Test_Signer_SignsAShortLivedAssertionTheKeySetVerifies(t *testing.T) {
	s := signer(t)
	now := time.Now()

	assertion, err := s.Assertion("ana", "Ana", now)
	require.NoError(t, err)
	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(assertion, claims, func(*jwt.Token) (any, error) { return publicKey(t, s), nil })

	require.NoError(t, err)
	require.Equal(t, "ana", claims["sub"])
	require.Equal(t, "ana", claims["email"])
	require.InDelta(t, now.Add(5*time.Minute).Unix(), claims["exp"], 1)
}

func Test_Identify_ReplacesForgedHeadersWithTheViewersAssertion(t *testing.T) {
	p := GrafanaProxy{signer: signer(t)}
	h := http.Header{}
	h.Set(AssertionHeader, "forged")
	h.Set("X-WEBAUTH-ROLE", "Admin")

	p.identify(h, requestctx.Viewer{Actor: actor.Actor{Login: "bob"}, Name: "Bob"})

	require.NotEqual(t, "forged", h.Get(AssertionHeader))
	require.Empty(t, h.Get("X-WEBAUTH-ROLE"))
	require.Empty(t, h.Get("X-WEBAUTH-USER"))
}

func Test_Identify_DropsAForgedAssertionWhenSigningIsOff(t *testing.T) {
	h := http.Header{}
	h.Set(AssertionHeader, "forged")

	GrafanaProxy{}.identify(h, requestctx.Viewer{Actor: actor.Actor{Login: "bob"}})

	require.Empty(t, h.Get(AssertionHeader))
}
