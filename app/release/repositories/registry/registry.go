package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/adrianozp/platform/pkg/apperror"
)

const manifestTypes = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json"

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

var errTagNotFound = apperror.NotFound("that tag does not exist in the image repository")

type Resolver struct {
	http *http.Client
	base func(host string) string
}

func NewResolver() Resolver {
	return Resolver{http: &http.Client{Timeout: 15 * time.Second}, base: func(host string) string { return "https://" + host }}
}

func WithBaseURL(baseURL string) Resolver {
	r := NewResolver()
	r.base = func(string) string { return baseURL }
	return r
}

func (r Resolver) Resolve(ctx context.Context, image, tag string) (string, error) {
	host, name := split(image)
	manifest := fmt.Sprintf("%s/v2/%s/manifests/%s", r.base(host), name, url.PathEscape(tag))
	resp, err := r.head(ctx, manifest, "")
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		resp, err = r.withToken(ctx, manifest, resp.Header.Get("WWW-Authenticate"))
	}
	if err != nil {
		return "", apperror.New(apperror.CodeUpstream, "the registry did not answer: "+err.Error())
	}
	return digestFrom(resp)
}

func digestFrom(resp *http.Response) (string, error) {
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", errTagNotFound
	case resp.StatusCode != http.StatusOK:
		return "", apperror.New(apperror.CodeUpstream, fmt.Sprintf("the registry answered %d", resp.StatusCode))
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if !strings.HasPrefix(digest, "sha256:") {
		return "", apperror.New(apperror.CodeUpstream, "the registry did not return a digest")
	}
	return digest, nil
}

func (r Resolver) head(ctx context.Context, target, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", manifestTypes)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	return resp, nil
}

func (r Resolver) withToken(ctx context.Context, target, challenge string) (*http.Response, error) {
	token, err := r.token(ctx, challenge)
	if err != nil {
		return nil, err
	}
	return r.head(ctx, target, token)
}

func (r Resolver) token(ctx context.Context, challenge string) (string, error) {
	params := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(challenge, -1) {
		params[m[1]] = m[2]
	}
	query := url.Values{"service": {params["service"]}, "scope": {params["scope"]}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params["realm"]+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	if body.Token != "" {
		return body.Token, err
	}
	return body.AccessToken, err
}

func split(image string) (string, string) {
	first, rest, found := strings.Cut(image, "/")
	if !found || (!strings.ContainsAny(first, ".:") && first != "localhost") {
		return dockerHub(image)
	}
	if first == "docker.io" || first == "index.docker.io" {
		return dockerHub(rest)
	}
	return first, rest
}

func dockerHub(name string) (string, string) {
	if !strings.Contains(name, "/") {
		name = "library/" + name
	}
	return "registry-1.docker.io", name
}
