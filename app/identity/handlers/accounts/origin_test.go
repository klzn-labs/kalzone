package accounts_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/identity/handlers/accounts"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/stretchr/testify/require"
)

func postWith(t *testing.T, headers map[string]string) int {
	t.Helper()
	return answerTo(t, headers).Code
}

func answerTo(t *testing.T, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	renderer, err := web.NewRenderer()
	require.NoError(t, err)
	engine := gin.New()
	engine.POST("/login", accounts.SameOrigin("https://kalzone.test/", renderer), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return serve(engine, req)
}

func Test_ARefusedPostNamesTheConfiguredAddress(t *testing.T) {
	refused := answerTo(t, map[string]string{"Origin": "http://192.168.1.10:8080"})

	require.Equal(t, http.StatusForbidden, refused.Code)
	require.Contains(t, refused.Body.String(), "Open the link at")
	require.Contains(t, refused.Body.String(), `href="https://kalzone.test"`)
}

func Test_SameOriginRefusesCrossSitePosts(t *testing.T) {
	refused := []map[string]string{
		{"Sec-Fetch-Site": "cross-site"},
		{"Sec-Fetch-Site": "same-site"},
		{"Origin": "https://evil.test"},
		{"Origin": "http://kalzone.test"},
		{"Origin": "null"},
		{"Sec-Fetch-Site": "same-origin", "Origin": "https://evil.test"},
	}
	for _, headers := range refused {
		require.Equal(t, http.StatusForbidden, postWith(t, headers), headers)
	}
}

func Test_SameOriginLetsOwnPagesAndPlainClientsThrough(t *testing.T) {
	accepted := []map[string]string{
		{},
		{"Sec-Fetch-Site": "same-origin", "Origin": "https://kalzone.test"},
		{"Sec-Fetch-Site": "none"},
		{"Origin": "https://KALZONE.test"},
	}
	for _, headers := range accepted {
		require.Equal(t, http.StatusNoContent, postWith(t, headers), headers)
	}
}
