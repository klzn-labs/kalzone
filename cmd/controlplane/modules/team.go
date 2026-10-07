package modules

import (
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/usecases/ownership"
	identityUsers "github.com/adrianozp/platform/app/identity/usecases/users"
	membersHandler "github.com/adrianozp/platform/app/team/handlers/members"
	teamsHandler "github.com/adrianozp/platform/app/team/handlers/teams"
	"github.com/adrianozp/platform/app/team/repositories/mysql"
	"github.com/adrianozp/platform/app/team/repositories/users"
	"github.com/adrianozp/platform/app/team/usecases/members"
	"github.com/adrianozp/platform/app/team/usecases/source"
	"github.com/adrianozp/platform/app/team/usecases/teams"
	"github.com/adrianozp/platform/internal/eventbus"
	"go.uber.org/fx"
)

func Team() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewTeamRepository, users.NewDirectory,
			teams.NewTeamsUsecase, members.NewMembersUsecase, source.NewLocalTeamSource,
			teamsHandler.NewTeamsHandler, membersHandler.NewMembersHandler,
		),
		teamInjections(),
		fx.Invoke(teamsHandler.RegisterTeamsRoutes, membersHandler.RegisterMembersRoutes),
	)
}

func teamInjections() fx.Option {
	return fx.Provide(
		func(r mysql.TeamRepository) teams.TeamStore { return r },
		func(r mysql.TeamRepository) members.MembershipStore { return r },
		func(r mysql.TeamRepository) members.TeamFinder { return r },
		func(r mysql.TeamRepository) source.MembershipReader { return r },
		func(u identityUsers.UsersUsecase) users.UserFinder { return u },
		func(d users.Directory) members.UserDirectory { return d },
		func(u ownership.OwnershipUsecase) teams.ServiceCounter { return u },
		func(p authz.Policy) teams.Authorizer { return p },
		func(p authz.Policy) members.Authorizer { return p },
		func(b *eventbus.Bus) teams.Publisher { return b },
		func(b *eventbus.Bus) members.Publisher { return b },
		func(u teams.TeamsUsecase) teamsHandler.Teams { return u },
		func(u members.MembersUsecase) membersHandler.Members { return u },
	)
}
