package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/config"
)

var ErrNoAccessToken = errors.New("github: no access token in response")

type Client struct {
	clientID     string
	clientSecret string
	oauthURL     string
	apiURL       string
	http         *http.Client
}

func NewClient(cfg config.Config) Client {
	return Client{
		clientID: cfg.GitHub.ClientID, clientSecret: cfg.GitHub.ClientSecret,
		oauthURL: strings.TrimRight(cfg.GitHub.OAuthURL, "/"), apiURL: strings.TrimRight(cfg.GitHub.APIURL, "/"),
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c Client) AuthorizeURL(state, redirectURI string) string {
	query := url.Values{"client_id": {c.clientID}, "redirect_uri": {redirectURI}, "state": {state}}
	return c.oauthURL + "/login/oauth/authorize?" + query.Encode()
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

func (c Client) Exchange(ctx context.Context, code, redirectURI string) (string, error) {
	form := url.Values{"client_id": {c.clientID}, "client_secret": {c.clientSecret}, "code": {code}, "redirect_uri": {redirectURI}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthURL+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var body tokenResponse
	if err := c.do(req, &body); err != nil {
		return "", err
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("%w: %s", ErrNoAccessToken, body.Error)
	}
	return body.AccessToken, nil
}

type userResponse struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func (c Client) CurrentUser(ctx context.Context, accessToken string) (entities.GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/user", nil)
	if err != nil {
		return entities.GitHubUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	var body userResponse
	if err := c.do(req, &body); err != nil {
		return entities.GitHubUser{}, err
	}
	return entities.GitHubUser{ID: body.ID, Login: body.Login, Name: body.Name, AvatarURL: body.AvatarURL}, nil
}

func (c Client) do(req *http.Request, out any) error {
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github: %s %s returned %d", req.Method, req.URL.Path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
