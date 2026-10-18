package githubapp

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const tokenMargin = time.Minute

var errNotConfigured = apperror.New(apperror.CodeNotConfigured, "builds are not configured: the GitHub App id and private key are missing")

type cachedToken struct {
	value     string
	expiresAt time.Time
}

type Client struct {
	appID  int64
	key    *rsa.PrivateKey
	apiURL string
	http   *http.Client
	mu     *sync.Mutex
	tokens map[string]cachedToken
}

func NewClient(cfg config.Config) (Client, error) {
	client := Client{
		appID: cfg.GitHub.AppID, apiURL: strings.TrimRight(cfg.GitHub.APIURL, "/"),
		http: &http.Client{Timeout: 15 * time.Second}, mu: &sync.Mutex{}, tokens: map[string]cachedToken{},
	}
	if cfg.GitHub.AppPrivateKey == "" {
		return client, nil
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(strings.ReplaceAll(cfg.GitHub.AppPrivateKey, `\n`, "\n")))
	client.key = key
	return client, err
}

func (c Client) Dispatch(ctx context.Context, r entities.BuildRequest) error {
	if c.appID == 0 || c.key == nil {
		return errNotConfigured
	}
	token, err := c.installationToken(ctx, r.Repository)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/repos/%s/actions/workflows/%s/dispatches", r.Repository, url.PathEscape(r.Workflow))
	body := map[string]any{"ref": r.Ref, "inputs": r.Inputs}
	return c.call(ctx, http.MethodPost, path, "token "+token, body, http.StatusNoContent, nil)
}

func (c Client) installationToken(ctx context.Context, repository string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.tokens[repository]; ok && time.Until(cached.expiresAt) > tokenMargin {
		return cached.value, nil
	}
	token, err := c.newInstallationToken(ctx, repository)
	if err != nil {
		return "", err
	}
	c.tokens[repository] = token
	return token.value, nil
}

func (c Client) newInstallationToken(ctx context.Context, repository string) (cachedToken, error) {
	appJWT, err := c.appJWT()
	if err != nil {
		return cachedToken{}, err
	}
	var installation struct {
		ID int64 `json:"id"`
	}
	if err := c.call(ctx, http.MethodGet, "/repos/"+repository+"/installation", "Bearer "+appJWT, nil, http.StatusOK, &installation); err != nil {
		return cachedToken{}, fmt.Errorf("the GitHub App is not installed on %s: %w", repository, err)
	}
	var token struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	path := fmt.Sprintf("/app/installations/%d/access_tokens", installation.ID)
	err = c.call(ctx, http.MethodPost, path, "Bearer "+appJWT, nil, http.StatusCreated, &token)
	return cachedToken{value: token.Token, expiresAt: token.ExpiresAt}, err
}

func (c Client) appJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:   fmt.Sprint(c.appID),
		IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(c.key)
}

func (c Client) call(ctx context.Context, method, path, auth string, body any, want int, out any) error {
	req, err := newRequest(ctx, method, c.apiURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", auth)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("github %s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func newRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}
