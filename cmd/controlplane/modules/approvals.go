package modules

import (
	requestsHandler "github.com/klzn-labs/kalzone/app/approvals/handlers/requests"
	approvalsDatabase "github.com/klzn-labs/kalzone/app/approvals/repositories/database"
	approvalsDeploy "github.com/klzn-labs/kalzone/app/approvals/repositories/deploy"
	approvalsConfig "github.com/klzn-labs/kalzone/app/approvals/repositories/envconfig"
	approvalsMysql "github.com/klzn-labs/kalzone/app/approvals/repositories/mysql"
	approvalsQueue "github.com/klzn-labs/kalzone/app/approvals/repositories/queue"
	"github.com/klzn-labs/kalzone/app/approvals/usecases/requests"
	"github.com/klzn-labs/kalzone/app/approvals/usecases/submit"
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/app/identity/usecases/tokens"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/internal/discord"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/httpserver/wait"
	"github.com/klzn-labs/kalzone/internal/sweeper"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"go.uber.org/fx"
)

func Approvals() fx.Option {
	return fx.Options(
		fx.Provide(
			approvalsMysql.NewRequestRepository, discord.NewNotifier, submit.NewSubmitUsecase, requests.NewRequestsUsecase,
			approvalsConfig.NewExecutor, approvalsDeploy.NewExecutor, approvalsQueue.NewExecutor, approvalsDatabase.NewExecutor, executors,
			requestsHandler.NewRequestsHandler,
		),
		approvalsInjections(),
		submitInjections(),
		fx.Invoke(requestsHandler.RegisterRequestsRoutes, startApprovalsSweeper),
	)
}

func approvalsInjections() fx.Option {
	return fx.Provide(
		func(r approvalsMysql.RequestRepository) submit.Store { return r },
		func(r approvalsMysql.RequestRepository) requests.Store { return r },
		func(n discord.Notifier) submit.Notifier { return n },
		func(b *eventbus.Bus) submit.Publisher { return b },
		func(b *eventbus.Bus) requests.Publisher { return b },
		func(p authz.Policy) requests.Authorizer { return p },
		func(u tokens.TokensUsecase) requests.TokenActors { return u },
		func(u configs.ConfigUsecase) approvalsConfig.Configs { return u },
		func(u deployments.DeploymentsUsecase) approvalsDeploy.Deployments { return u },
		func(u queues.QueuesUsecase) approvalsQueue.Queues { return u },
		func(u databases.DatabasesUsecase) approvalsDatabase.Databases { return u },
		func(u requests.RequestsUsecase) requestsHandler.Requests { return u },
		func(u requests.RequestsUsecase) wait.Requests { return u },
	)
}

func submitInjections() fx.Option {
	return fx.Provide(
		func(u submit.SubmitUsecase) configs.Requests { return u },
		func(u submit.SubmitUsecase) deployments.Requests { return u },
		func(u submit.SubmitUsecase) queues.Requests { return u },
		func(u submit.SubmitUsecase) databases.Requests { return u },
	)
}

func executors(c approvalsConfig.Executor, d approvalsDeploy.Executor, q approvalsQueue.Executor, db approvalsDatabase.Executor) requests.Executors {
	return requests.Executors{
		approval.KindConfigRevise: c, approval.KindSecretSet: c, approval.KindSecretUnset: c,
		approval.KindDeploy: d, approval.KindPromote: d, approval.KindRollback: d, approval.KindConfigApply: d, approval.KindRemove: d,
		approval.KindQueueCreate: q, approval.KindQueueUpdate: q, approval.KindQueueDelete: q,
		approval.KindDatabaseCreate: db, approval.KindDatabaseAttach: db, approval.KindDatabaseDetach: db, approval.KindDatabaseDelete: db,
	}
}

func startApprovalsSweeper(lc fx.Lifecycle, u requests.RequestsUsecase) {
	sweeper.Start(lc, "approvals", u)
}
