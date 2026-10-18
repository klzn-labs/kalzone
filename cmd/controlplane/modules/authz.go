package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
	"github.com/klzn-labs/kalzone/app/team/usecases/source"
	"go.uber.org/fx"
)

func Authz() fx.Option {
	return fx.Provide(
		authz.NewPolicy,
		func(s source.LocalTeamSource) authz.MembershipReader { return s },
		func(u environments.EnvironmentsUsecase) authz.ProtectionReader { return u },
	)
}
