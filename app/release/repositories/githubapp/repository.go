package githubapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type PullRequest struct {
	Branch  string
	Path    string
	Content string
	Message string
	Title   string
	Body    string
}

type OpenedPullRequest struct {
	URL    string
	Number int
	Branch string
}

func (c Client) Installed(ctx context.Context, repository string) (bool, error) {
	if c.appID == 0 || c.key == nil {
		return false, errNotConfigured
	}
	appJWT, err := c.appJWT()
	if err != nil {
		return false, err
	}
	status, _, err := c.do(ctx, http.MethodGet, "/repos/"+repository+"/installation", "Bearer "+appJWT, nil)
	if err != nil || status == http.StatusNotFound {
		return false, err
	}
	return status == http.StatusOK, unexpected(status, http.StatusOK, "installation")
}

func (c Client) DefaultBranch(ctx context.Context, repository string) (string, error) {
	var repo struct {
		DefaultBranch string `json:"default_branch"`
	}
	err := c.installed(ctx, repository, http.MethodGet, "/repos/"+repository, nil, http.StatusOK, &repo)
	return repo.DefaultBranch, err
}

func (c Client) File(ctx context.Context, repository, path, ref string) (string, bool, error) {
	token, err := c.installationToken(ctx, repository)
	if err != nil {
		return "", false, err
	}
	status, raw, err := c.do(ctx, http.MethodGet, contentsPath(repository, path)+"?ref="+url.QueryEscape(ref), "token "+token, nil)
	if err != nil || status == http.StatusNotFound {
		return "", false, err
	}
	if err := unexpected(status, http.StatusOK, "contents"); err != nil {
		return "", false, err
	}
	return decodeContent(raw)
}

func decodeContent(raw []byte) (string, bool, error) {
	var file struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return "", false, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	return string(decoded), err == nil, err
}

func (c Client) OpenPullRequest(ctx context.Context, repository, base string, pr PullRequest) (OpenedPullRequest, error) {
	if open, found, err := c.openPull(ctx, repository, pr.Branch); err != nil || found {
		return open, err
	}
	if err := c.branch(ctx, repository, base, pr.Branch); err != nil {
		return OpenedPullRequest{}, err
	}
	body := map[string]any{"message": pr.Message, "content": base64.StdEncoding.EncodeToString([]byte(pr.Content)), "branch": pr.Branch}
	if err := c.installed(ctx, repository, http.MethodPut, contentsPath(repository, pr.Path), body, http.StatusCreated, nil); err != nil {
		return OpenedPullRequest{}, err
	}
	var opened pullJSON
	request := map[string]string{"title": pr.Title, "head": pr.Branch, "base": base, "body": pr.Body}
	err := c.installed(ctx, repository, http.MethodPost, "/repos/"+repository+"/pulls", request, http.StatusCreated, &opened)
	return opened.toOpened(), err
}

type pullJSON struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Head    struct {
		Ref string `json:"ref"`
	} `json:"head"`
}

func (p pullJSON) toOpened() OpenedPullRequest {
	return OpenedPullRequest{URL: p.HTMLURL, Number: p.Number, Branch: p.Head.Ref}
}

func (c Client) openPull(ctx context.Context, repository, branch string) (OpenedPullRequest, bool, error) {
	owner := strings.Split(repository, "/")[0]
	var pulls []pullJSON
	path := "/repos/" + repository + "/pulls?state=open&head=" + url.QueryEscape(owner+":"+branch)
	if err := c.installed(ctx, repository, http.MethodGet, path, nil, http.StatusOK, &pulls); err != nil || len(pulls) == 0 {
		return OpenedPullRequest{}, false, err
	}
	return pulls[0].toOpened(), true, nil
}

func (c Client) branch(ctx context.Context, repository, base, branch string) error {
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := c.installed(ctx, repository, http.MethodGet, "/repos/"+repository+"/git/ref/heads/"+base, nil, http.StatusOK, &ref); err != nil {
		return err
	}
	token, err := c.installationToken(ctx, repository)
	if err != nil {
		return err
	}
	body := map[string]string{"ref": "refs/heads/" + branch, "sha": ref.Object.SHA}
	status, _, err := c.do(ctx, http.MethodPost, "/repos/"+repository+"/git/refs", "token "+token, body)
	if err != nil || status == http.StatusUnprocessableEntity {
		return err
	}
	return unexpected(status, http.StatusCreated, "create branch")
}

func (c Client) installed(ctx context.Context, repository, method, path string, body any, want int, out any) error {
	token, err := c.installationToken(ctx, repository)
	if err != nil {
		return err
	}
	return c.call(ctx, method, path, "token "+token, body, want, out)
}

func (c Client) do(ctx context.Context, method, path, auth string, body any) (int, []byte, error) {
	req, err := newRequest(ctx, method, c.apiURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", auth)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, err
}

func contentsPath(repository, path string) string {
	return "/repos/" + repository + "/contents/" + path
}

func unexpected(status, want int, what string) error {
	if status == want {
		return nil
	}
	return fmt.Errorf("github %s: %d", what, status)
}
