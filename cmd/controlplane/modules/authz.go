package modules

import (
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/usecases/environments"
	"github.com/adrianozp/platform/app/team/usecases/source"
	"github.com/adrianozp/platform/internal/eventbus"
	"go.uber.org/fx"
)

func Authz() fx.Option {
	return fx.Provide(
		authz.NewPolicy,
		func(s source.LocalTeamSource) authz.MembershipReader { return s },
		func(u environments.EnvironmentsUsecase) authz.ProtectionReader { return u },
		func(b *eventbus.Bus) authz.Publisher { return b },
	)
}
