package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/klzn-labs/kalzone/internal/fakegithub"
	"github.com/stretchr/testify/require"
)

type Client struct {
	suite     *Suite
	http      *http.Client
	CSRFToken string
	bearer    string
}

type Response struct {
	Status       int
	Body         string
	Location     string
	CacheControl string
	Header       http.Header
}

func (s *Suite) Anonymous(t *testing.T) *Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{suite: s, http: &http.Client{Jar: jar, CheckRedirect: noRedirect}}
}

func (s *Suite) WithToken(t *testing.T, token string) *Client {
	t.Helper()
	c := s.Anonymous(t)
	c.bearer = token
	return c
}

func (c *Client) UseToken(token string) {
	c.bearer = token
}

func (s *Suite) SignIn(t *testing.T, login string) *Client {
	t.Helper()
	s.nextID++
	code := "code-" + login
	s.GitHub.WillSignIn(code, fakegithub.User{ID: s.nextID, Login: login, Name: strings.ToUpper(login), AvatarURL: ""})
	c := s.Anonymous(t)
	start := c.Do(t, http.MethodGet, "/auth/github", nil)
	state := queryParam(t, start.Location, "state")
	callback := c.Do(t, http.MethodGet, "/auth/github/callback?code="+code+"&state="+state, nil)
	require.Equal(t, http.StatusSeeOther, callback.Status, callback.Body)
	require.Equal(t, "/", callback.Location)
	var me struct {
		CSRFToken string `json:"csrf_token"`
	}
	c.MustJSON(t, http.MethodGet, "/api/v1/me", nil, http.StatusOK, &me)
	c.CSRFToken = me.CSRFToken
	return c
}

func (s *Suite) Guest(t *testing.T) *Client {
	t.Helper()
	c := s.Anonymous(t)
	entered := c.Do(t, http.MethodPost, "/demo", nil)
	require.Equal(t, http.StatusSeeOther, entered.Status, entered.Body)
	require.Equal(t, "/", entered.Location)
	var me struct {
		CSRFToken string `json:"csrf_token"`
	}
	c.MustJSON(t, http.MethodGet, "/api/v1/me", nil, http.StatusOK, &me)
	c.CSRFToken = me.CSRFToken
	return c
}

func queryParam(t *testing.T, raw, name string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed.Query().Get(name)
}

func (c *Client) Do(t *testing.T, method, path string, body any) Response {
	t.Helper()
	return c.Send(t, method, path, "application/json", body)
}

func (c *Client) Send(t *testing.T, method, path, contentType string, body any) Response {
	t.Helper()
	req, err := http.NewRequest(method, c.suite.Server.URL+path, encode(t, body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	if c.CSRFToken != "" {
		req.Header.Set("X-CSRF-Token", c.CSRFToken)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	response := send(t, c.http, req)
	c.suite.contract.check(t, req, response)
	return response
}

func send(t *testing.T, client *http.Client, req *http.Request) Response {
	t.Helper()
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return Response{
		Status: resp.StatusCode, Body: string(raw), Location: resp.Header.Get("Location"),
		CacheControl: resp.Header.Get("Cache-Control"), Header: resp.Header,
	}
}

func (c *Client) MustJSON(t *testing.T, method, path string, body any, status int, out any) Response {
	t.Helper()
	resp := c.Do(t, method, path, body)
	require.Equal(t, status, resp.Status, "%s %s: %s", method, path, resp.Body)
	if out != nil {
		require.NoError(t, json.Unmarshal([]byte(resp.Body), out))
	}
	return resp
}

func (c *Client) Form(t *testing.T, path string, values url.Values) Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.suite.Server.URL+path, strings.NewReader(values.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := c.http.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return Response{Status: resp.StatusCode, Body: string(raw), Location: resp.Header.Get("Location"), CacheControl: resp.Header.Get("Cache-Control")}
}

func encode(t *testing.T, body any) io.Reader {
	t.Helper()
	if body == nil {
		return nil
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return bytes.NewReader(raw)
}

func (c *Client) Cookies(u *url.URL) []*http.Cookie {
	return c.http.Jar.Cookies(u)
}

func (s *Suite) Raw(t *testing.T, method, path string, body []byte, headers map[string]string) Response {
	t.Helper()
	req, err := http.NewRequest(method, s.Server.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	response := send(t, http.DefaultClient, req)
	s.contract.check(t, req, response)
	return response
}

func (s *Suite) Agent(t *testing.T, method, path string, body any) Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	if body == nil {
		raw = nil
	}
	return s.Raw(t, method, path, raw, map[string]string{
		"Authorization": "Bearer " + AgentToken, "X-Agent-Name": "vps-1", "X-Agent-Version": "test", "Content-Type": "application/json",
	})
}

func (s *Suite) Queued(t *testing.T, method, path string, body any) Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	if body == nil {
		raw = nil
	}
	return s.Raw(t, method, path, raw, map[string]string{"Authorization": "Bearer " + QueuedToken, "Content-Type": "application/json"})
}
