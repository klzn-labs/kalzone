package modules

import (
	"context"
	"log"

	"github.com/klzn-labs/kalzone/app/authz"
	accountsHandler "github.com/klzn-labs/kalzone/app/identity/handlers/accounts"
	"github.com/klzn-labs/kalzone/app/identity/handlers/auth"
	"github.com/klzn-labs/kalzone/app/identity/handlers/guard"
	"github.com/klzn-labs/kalzone/app/identity/handlers/me"
	tokensHandler "github.com/klzn-labs/kalzone/app/identity/handlers/tokens"
	usersHandler "github.com/klzn-labs/kalzone/app/identity/handlers/users"
	"github.com/klzn-labs/kalzone/app/identity/repositories/github"
	"github.com/klzn-labs/kalzone/app/identity/repositories/mysql"
	identityTeams "github.com/klzn-labs/kalzone/app/identity/repositories/teams"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/app/identity/usecases/sessions"
	"github.com/klzn-labs/kalzone/app/identity/usecases/signin"
	"github.com/klzn-labs/kalzone/app/identity/usecases/tokens"
	"github.com/klzn-labs/kalzone/app/identity/usecases/users"
	"github.com/klzn-labs/kalzone/app/team/usecases/source"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/sweeper"
	"go.uber.org/fx"
)

func Identity() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewUserRepository, mysql.NewSessionRepository, mysql.NewTokenRepository, github.NewClient,
			signin.NewSignInUsecase, sessions.NewSessionsUsecase, users.NewUsersUsecase, tokens.NewTokensUsecase,
			identityTeams.NewLookup, auth.NewAuthHandler, usersHandler.NewUsersHandler, guard.NewGuards,
			tokensHandler.NewTokensHandler, auth.NewDemoHandler,
		),
		identityInjections(),
		tokenInjections(),
		Accounts(),
		fx.Invoke(auth.RegisterAuthRoutes, auth.RegisterDemoRoutes, startSessionSweeper, me.RegisterMeRoutes, usersHandler.RegisterUsersRoutes, tokensHandler.RegisterTokensRoutes),
	)
}

func Accounts() fx.Option {
	return fx.Options(
		fx.Provide(
			accounts.NewMode, mysql.NewAccountLinkRepository, mysql.NewAccountRepository,
			accounts.NewSetupUsecase, accounts.NewPasswordSignIn, accounts.NewInvitesUsecase, accounts.NewPeopleUsecase,
			accountsHandler.NewPagesHandler, accountsHandler.NewInvitesHandler,
			func() accounts.Logger { return log.Default() },
		),
		accountStoreInjections(),
		accountHandlerInjections(),
		fx.Invoke(accountsHandler.RegisterPagesRoutes, accountsHandler.RegisterInvitesRoutes, ensureSetupLinkOnStart),
	)
}

func accountStoreInjections() fx.Option {
	return fx.Provide(
		func(r mysql.AccountLinkRepository) accounts.LinkStore { return r },
		func(r mysql.AccountLinkRepository) accounts.InviteLinks { return r },
		func(r mysql.AccountRepository) accounts.AccountStore { return r },
		func(r mysql.AccountRepository) accounts.InviteAccounts { return r },
		func(r mysql.AccountRepository) accounts.PeopleAccounts { return r },
		func(r mysql.UserRepository) accounts.UserStore { return r },
		func(r mysql.UserRepository) accounts.PeopleUsers { return r },
		func(r mysql.SessionRepository) accounts.SessionStore { return r },
		func(u tokens.TokensUsecase) accounts.TokenIssuer { return u },
		func(l identityTeams.Lookup) accounts.TeamLookup { return l },
		func(p authz.Policy) accounts.Authorizer { return p },
		func(b *eventbus.Bus) accounts.Publisher { return b },
		func(p accounts.PasswordSignIn) accounts.FailureClearer { return p },
	)
}

func accountHandlerInjections() fx.Option {
	return fx.Provide(
		func(u accounts.SetupUsecase) accountsHandler.Setup { return u },
		func(u accounts.InvitesUsecase) accountsHandler.InviteAccepter { return u },
		func(u accounts.InvitesUsecase) accountsHandler.Invites { return u },
		func(u accounts.PeopleUsecase) accountsHandler.PasswordResetter { return u },
		func(p accounts.PasswordSignIn) accountsHandler.PasswordSignIner { return p },
	)
}

func ensureSetupLinkOnStart(lc fx.Lifecycle, u accounts.SetupUsecase) {
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error { return u.EnsureSetupLink(ctx) }})
}

func tokenInjections() fx.Option {
	return fx.Provide(
		func(r mysql.TokenRepository) tokens.TokenStore { return r },
		func(r mysql.UserRepository) tokens.UserReader { return r },
		func(l identityTeams.Lookup) tokens.TeamLookup { return l },
		func(l identityTeams.Lookup) tokens.TeamScope { return l },
		func(l identityTeams.Lookup) sessions.TeamScope { return l },
		func(s source.LocalTeamSource) identityTeams.MemberTeams { return s },
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
		func(l identityTeams.Lookup) users.CoMembers { return l },
		func(p authz.Policy) users.Authorizer { return p },
		func(b *eventbus.Bus) users.Publisher { return b },
		func(u signin.SignInUsecase) auth.SignIner { return u },
		func(u sessions.SessionsUsecase) auth.SignOuter { return u },
		func(u sessions.SessionsUsecase) guard.Authenticator { return u },
		func(u sessions.SessionsUsecase) auth.DemoEnterer { return u },
		func(u users.UsersUsecase) usersHandler.Users { return u },
	)
}

func startSessionSweeper(lc fx.Lifecycle, u sessions.SessionsUsecase) {
	sweeper.Start(lc, "sessions", u)
}
