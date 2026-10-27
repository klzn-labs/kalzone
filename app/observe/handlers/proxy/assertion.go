package proxy

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const (
	AssertionHeader = "X-JWT-Assertion"
	assertionTTL    = 5 * time.Minute
)

var errNotEd25519 = errors.New("grafana.jwt_key must be a base64 PEM of a PKCS#8 Ed25519 private key")

type Signer struct {
	key ed25519.PrivateKey
	kid string
}

func NewSigner(cfg config.Config) (Signer, error) {
	if cfg.Grafana.JWTKey == "" {
		return Signer{}, nil
	}
	key, err := parseKey(cfg.Grafana.JWTKey)
	if err != nil {
		return Signer{}, err
	}
	sum := sha256.Sum256(key.Public().(ed25519.PublicKey))
	return Signer{key: key, kid: hex.EncodeToString(sum[:8])}, nil
}

func parseKey(encoded string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errNotEd25519
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errNotEd25519
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	key, ok := parsed.(ed25519.PrivateKey)
	if err != nil || !ok {
		return nil, errNotEd25519
	}
	return key, nil
}

func (s Signer) Enabled() bool {
	return s.key != nil
}

func (s Signer) Assertion(login, name string, now time.Time) (string, error) {
	claims := jwt.MapClaims{"sub": login, "email": login, "name": name, "iat": now.Unix(), "exp": now.Add(assertionTTL).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.kid
	return token.SignedString(s.key)
}

func (s Signer) KeySet() map[string]any {
	public := s.key.Public().(ed25519.PublicKey)
	key := map[string]string{"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "kid": s.kid, "x": base64.RawURLEncoding.EncodeToString(public)}
	return map[string]any{"keys": []map[string]string{key}}
}
