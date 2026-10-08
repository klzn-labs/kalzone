package signing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adrianozp/platform-sdk-go/queue"
	"github.com/adrianozp/platform/internal/queued/signing"
	"github.com/stretchr/testify/require"
)

func Test_TheSDKAcceptsWhatTheGatewaySigns(t *testing.T) {
	body := `{"city":7}`
	var got queue.Message
	h := queue.Handler("key", func(_ context.Context, m queue.Message) error { got = m; return nil })
	req := httptest.NewRequest(http.MethodPost, "/hooks", strings.NewReader(body))
	req.Header.Set("Platform-Signature", signing.Sign("key", time.Now(), []byte(body)))
	req.Header.Set("Platform-Attempt", "2")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 2, got.Attempt)
}

func Test_TheGatewayAcceptsWhatTheSDKSigns(t *testing.T) {
	header := queue.Sign("key", time.Now(), []byte("x"))

	require.NoError(t, signing.Verify(header, []string{"key"}, []byte("x"), time.Now(), time.Minute))
}
