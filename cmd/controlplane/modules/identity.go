package modules

import (
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/identity/handlers/auth"
	"github.com/adrianozp/platform/app/identity/handlers/guard"
	"github.com/adrianozp/platform/app/identity/handlers/me"
	tokensHandler "github.com/adrianozp/platform/app/identity/handlers/tokens"
	usersHandler "github.com/adrianozp/platform/app/identity/handlers/users"
	"github.com/adrianozp/platform/app/identity/repositories/github"
	"github.com/adrianozp/platform/app/identity/repositories/mysql"
	identityTeams "github.com/adrianozp/platform/app/identity/repositories/teams"
	"github.com/adrianozp/platform/app/identity/usecases/sessions"
	"github.com/adrianozp/platform/app/identity/usecases/signin"
	"github.com/adrianozp/platform/app/identity/usecases/tokens"
	"github.com/adrianozp/platform/app/identity/usecases/users"
	"github.com/adrianozp/platform/app/team/usecases/teams"
	"github.com/adrianozp/platform/internal/eventbus"
	"go.uber.org/fx"
)

func Identity() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewUserRepository, mysql.NewSessionRepository, mysql.NewTokenRepository, github.NewClient,
			signin.NewSignInUsecase, sessions.NewSessionsUsecase, users.NewUsersUsecase, tokens.NewTokensUsecase,
			identityTeams.NewLookup, auth.NewAuthHandler, usersHandler.NewUsersHandler, guard.NewGuards,
			tokensHandler.NewTokensHandler,
		),
		identityInjections(),
		tokenInjections(),
		fx.Invoke(auth.RegisterAuthRoutes, me.RegisterMeRoutes, usersHandler.RegisterUsersRoutes, tokensHandler.RegisterTokensRoutes),
	)
}

func tokenInjections() fx.Option {
	return fx.Provide(
		func(r mysql.TokenRepository) tokens.TokenStore { return r },
		func(r mysql.UserRepository) tokens.UserReader { return r },
		func(l identityTeams.Lookup) tokens.TeamLookup { return l },
		func(u teams.TeamsUsecase) identityTeams.TeamFinder { return u },
		func(p authz.Policy) tokens.Authorizer { return p },
		func(b *eventbus.Bus) tokens.Publisher { return b },
		func(u tokens.TokensUsecase) guard.TokenAuthenticator { return u },
		func(u tokens.TokensUsecase) tokensHandler.Tokens { return u },
	)
}

func identityInjections() fx.Option {
	return fx.Provide(
		func(c github.Client) signin.GitHub { return c },
		func(c github.Client) auth.SignInStarter { return c },
		func(r mysql.UserRepository) signin.UserStore { return r },
		func(r mysql.SessionRepository) signin.SessionStore { return r },
		func(r mysql.SessionRepository) sessions.SessionStore { return r },
		func(r mysql.UserRepository) sessions.UserReader { return r },
		func(r mysql.UserRepository) users.UserStore { return r },
		func(p authz.Policy) users.Authorizer { return p },
		func(b *eventbus.Bus) users.Publisher { return b },
		func(u signin.SignInUsecase) auth.SignIner { return u },
		func(u sessions.SessionsUsecase) auth.SignOuter { return u },
		func(u sessions.SessionsUsecase) guard.Authenticator { return u },
		func(u users.UsersUsecase) usersHandler.Users { return u },
	)
}
