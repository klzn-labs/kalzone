package guard

import (
	"context"
	"strings"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/gin-gonic/gin"
)

const bearerScheme = "bearer "

type TokenAuthenticator interface {
	Authenticate(ctx context.Context, raw, ip string) (entities.TokenAuthenticated, error)
}

var errBearerOnPages = apperror.New(apperror.CodeUnauthenticated, "access tokens work on /api/v1 and /mcp only, sign in to use the website")

var readFree = map[string]bool{"/api/v1/whoami": true, "/api/v1/me": true, "/api/v1/tokens": true, "/api/v1/openapi.json": true}

func bearer(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if len(header) < len(bearerScheme) || !strings.EqualFold(header[:len(bearerScheme)], bearerScheme) {
		return "", false
	}
	return strings.TrimSpace(header[len(bearerScheme):]), true
}

var errNoBearer = apperror.New(apperror.CodeUnauthenticated, "an access token is required").
	WithHint("send Authorization: Bearer <token>, a token from the Tokens page")

func authenticateToken(c *gin.Context, tokens TokenAuthenticator, raw string) error {
	a, err := signInToken(c, tokens, raw)
	if err != nil {
		return err
	}
	if isSafe(c.Request.Method) && !readFree[c.FullPath()] {
		return authz.CanRead(a)
	}
	return nil
}

func signInToken(c *gin.Context, tokens TokenAuthenticator, raw string) (actor.Actor, error) {
	authenticated, err := tokens.Authenticate(c.Request.Context(), raw, c.ClientIP())
	if err != nil {
		return actor.Actor{}, err
	}
	a := authenticated.Actor()
	requestctx.SetViewer(c, requestctx.Viewer{Actor: a, Name: authenticated.User.Name, AvatarURL: authenticated.User.AvatarURL})
	return a, nil
}

func tokenGuard(tokens TokenAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearer(c)
		var err error = errNoBearer
		if ok && raw != "" {
			_, err = signInToken(c, tokens, raw)
		}
		if err != nil {
			c.Header("WWW-Authenticate", "Bearer")
			httperror.Abort(c, err)
		}
	}
}
