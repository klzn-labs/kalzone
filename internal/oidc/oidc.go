package oidc

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/adrianozp/platform/pkg/config"
	"github.com/golang-jwt/jwt/v5"
)

const (
	keysTTL         = time.Hour
	minRefreshDelay = time.Minute
)

var ErrUnknownKey = errors.New("oidc: token signed with an unknown key")

type Claims struct {
	jwt.RegisteredClaims
	Repository  string `json:"repository"`
	WorkflowRef string `json:"workflow_ref"`
	RunID       string `json:"run_id"`
}

type Verifier struct {
	issuer    string
	audience  string
	http      *http.Client
	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewVerifier(cfg config.Config) *Verifier {
	return &Verifier{
		issuer: strings.TrimRight(cfg.OIDC.Issuer, "/"), audience: cfg.OIDCAudience(),
		http: &http.Client{Timeout: 10 * time.Second}, keys: map[string]*rsa.PublicKey{},
	}
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(raw, &claims, v.keyFor(ctx),
		jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(), jwt.WithLeeway(30*time.Second))
	if err != nil {
		return Claims{}, err
	}
	return claims, nil
}

func (v *Verifier) keyFor(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		return v.key(ctx, kid)
	}
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	age := time.Since(v.fetchedAt)
	if key, ok := v.keys[kid]; ok && age < keysTTL {
		return key, nil
	}
	if v.fetchedAt.IsZero() || age > minRefreshDelay {
		if err := v.refresh(ctx); err != nil {
			return nil, err
		}
	}
	return v.lookup(kid)
}

func (v *Verifier) lookup(kid string) (*rsa.PublicKey, error) {
	if key, ok := v.keys[kid]; ok {
		return key, nil
	}
	return nil, ErrUnknownKey
}

func (v *Verifier) refresh(ctx context.Context) error {
	var discovery struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := v.getJSON(ctx, v.issuer+"/.well-known/openid-configuration", &discovery); err != nil {
		return err
	}
	var set jwks
	if err := v.getJSON(ctx, discovery.JWKSURI, &set); err != nil {
		return err
	}
	v.keys, v.fetchedAt = set.rsaKeys(), time.Now()
	return nil
}

func (v *Verifier) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc: GET %s returned %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

func (s jwks) rsaKeys() map[string]*rsa.PublicKey {
	keys := map[string]*rsa.PublicKey{}
	for _, k := range s.Keys {
		if key, err := k.rsa(); err == nil {
			keys[k.Kid] = key
		}
	}
	return keys
}

func (k jwk) rsa() (*rsa.PublicKey, error) {
	if k.Kty != "RSA" {
		return nil, errors.New("oidc: not an RSA key")
	}
	n, nErr := base64.RawURLEncoding.DecodeString(k.N)
	e, eErr := base64.RawURLEncoding.DecodeString(k.E)
	if err := errors.Join(nErr, eErr); err != nil {
		return nil, err
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
}
