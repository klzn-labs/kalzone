package accounts_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/handlers/accounts"
	usecase "github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

type setup struct {
	kindErr error
}

func (s setup) LinkKind(context.Context, string) (entities.LinkKind, error) {
	return entities.LinkSetup, s.kindErr
}

func (setup) Complete(context.Context, string, usecase.SetupInput) (usecase.SetupResult, error) {
	return usecase.SetupResult{AgentToken: "kz_agent_secret", MCPCommand: "claude mcp add kalzone"}, nil
}

func pagesEngine(t *testing.T, s setup) *gin.Engine {
	t.Helper()
	return signInEngine(t, s, nil)
}

func signInEngine(t *testing.T, s setup, signIn accounts.PasswordSignIner) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	renderer, err := web.NewRenderer()
	require.NoError(t, err)
	cfg := config.Config{BaseURL: "https://kalzone.test"}
	h := accounts.NewPagesHandler(cfg, usecase.NewMode(cfg), renderer, s, nil, nil, signIn, nil)
	engine := gin.New()
	accounts.RegisterPagesRoutes(httpserver.Public{RouterGroup: &engine.RouterGroup}, h)
	return engine
}

func serve(engine *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func Test_TheSetupDonePageIsNeverCached(t *testing.T) {
	form := url.Values{"username": {"ana"}, "password": {"correct horse battery"}, "confirm": {"correct horse battery"}}
	req := httptest.NewRequest(http.MethodPost, "/join/tok", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := serve(pagesEngine(t, setup{}), req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Contains(t, rec.Body.String(), "kz_agent_secret")
}

func Test_AFailedLinkLookupIsNotCalledAnInvalidLink(t *testing.T) {
	rec := serve(pagesEngine(t, setup{kindErr: errors.New("db down")}), httptest.NewRequest(http.MethodGet, "/join/tok", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "Something went wrong")
	require.NotContains(t, rec.Body.String(), "no longer valid")
}

func Test_ACrossSiteSetupPostIsRefused(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/join/tok", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	require.Equal(t, http.StatusForbidden, serve(pagesEngine(t, setup{}), req).Code)
}

type failingSignIn struct{ err error }

func (f failingSignIn) SignIn(context.Context, string, string, string) (entities.IssuedSession, error) {
	return entities.IssuedSession{}, f.err
}

func signInAnswer(t *testing.T, err error) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"username": {"ana"}, "password": {"secret-password"}, "next": {"/services"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return serve(signInEngine(t, setup{}, failingSignIn{err: err}), req)
}

func Test_AFailedSignInKeepsNextButNotTheUsername(t *testing.T) {
	wrong := signInAnswer(t, apperror.New(apperror.CodeSignInFailed, "Wrong username or password."))
	throttled := signInAnswer(t, usecase.ErrThrottled)
	broken := signInAnswer(t, errors.New("database is down"))

	require.Equal(t, http.StatusSeeOther, wrong.Code)
	require.Equal(t, "/login?error=password&next=%2Fservices", wrong.Header().Get("Location"))
	require.Equal(t, "/login?error=throttled&next=%2Fservices", throttled.Header().Get("Location"))
	require.Equal(t, "/login?error=unavailable&next=%2Fservices", broken.Header().Get("Location"))
}
