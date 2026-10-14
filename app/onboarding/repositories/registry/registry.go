package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

var (
	errNoCredentials = errors.New("the platform holds no registry credentials")
	errNotDockerHub  = errors.New("only Docker Hub repositories are checked")
)

type Registry struct {
	authURL  string
	username string
	token    string
	http     *http.Client
}

func New(authURL, username, token string) Registry {
	return Registry{authURL: authURL, username: username, token: token, http: &http.Client{Timeout: 10 * time.Second}}
}

type access struct {
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

func (r Registry) CanPush(ctx context.Context, imageRepository string) (bool, error) {
	name, err := dockerHubName(imageRepository)
	if err != nil {
		return false, err
	}
	if r.username == "" || r.token == "" {
		return false, errNoCredentials
	}
	granted, err := r.grants(ctx, name)
	if err != nil {
		return false, err
	}
	for _, a := range granted {
		if a.Name == name && slices.Contains(a.Actions, "push") {
			return true, nil
		}
	}
	return false, nil
}

func dockerHubName(imageRepository string) (string, error) {
	name := strings.TrimPrefix(imageRepository, "docker.io/")
	first := strings.Split(name, "/")[0]
	if strings.ContainsAny(first, ".:") {
		return "", errNotDockerHub
	}
	return name, nil
}

func (r Registry) grants(ctx context.Context, name string) ([]access, error) {
	query := url.Values{"service": {"registry.docker.io"}, "scope": {"repository:" + name + ":push,pull"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.authURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(r.username, r.token)
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the registry refused the platform's credentials: %d", resp.StatusCode)
	}
	return decodeGrants(resp)
}

func decodeGrants(resp *http.Response) ([]access, error) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	parts := strings.Split(body.Token, ".")
	if len(parts) != 3 {
		return nil, errors.New("the registry answered a token that is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims struct {
		Access []access `json:"access"`
	}
	return claims.Access, json.Unmarshal(payload, &claims)
}
