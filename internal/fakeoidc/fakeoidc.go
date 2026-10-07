package fakeoidc

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const keyID = "fake-key"

type Issuer struct {
	*httptest.Server
	key *rsa.PrivateKey
}

func New() *Issuer {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	issuer := &Issuer{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", issuer.discovery)
	mux.HandleFunc("GET /.well-known/jwks", issuer.jwks)
	issuer.Server = httptest.NewServer(mux)
	return issuer
}

type Token struct {
	Audience   string
	Repository string
	Expires    time.Duration
	Issuer     string
	Key        *rsa.PrivateKey
}

func (i *Issuer) Mint(t Token) string {
	claims := jwt.MapClaims{
		"iss": firstNonEmpty(t.Issuer, i.URL), "aud": t.Audience, "repository": t.Repository,
		"iat": time.Now().Unix(), "nbf": time.Now().Unix(), "exp": time.Now().Add(t.Expires).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	key := t.Key
	if key == nil {
		key = i.key
	}
	signed, err := token.SignedString(key)
	if err != nil {
		panic(err)
	}
	return signed
}

func (i *Issuer) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"issuer": i.URL, "jwks_uri": i.URL + "/.well-known/jwks"})
}

func (i *Issuer) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := i.key.PublicKey
	writeJSON(w, map[string]any{"keys": []map[string]string{{
		"kid": keyID, "kty": "RSA", "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
