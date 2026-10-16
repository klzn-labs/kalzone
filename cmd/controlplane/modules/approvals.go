package modules

import (
	requestsHandler "github.com/adrianozp/platform/app/approvals/handlers/requests"
	approvalsDeploy "github.com/adrianozp/platform/app/approvals/repositories/deploy"
	approvalsConfig "github.com/adrianozp/platform/app/approvals/repositories/envconfig"
	approvalsMysql "github.com/adrianozp/platform/app/approvals/repositories/mysql"
	approvalsQueue "github.com/adrianozp/platform/app/approvals/repositories/queue"
	"github.com/adrianozp/platform/app/approvals/usecases/requests"
	"github.com/adrianozp/platform/app/approvals/usecases/submit"
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/app/envconfig/usecases/configs"
	"github.com/adrianozp/platform/app/identity/usecases/tokens"
	"github.com/adrianozp/platform/app/queue/usecases/queues"
	"github.com/adrianozp/platform/internal/discord"
	"github.com/adrianozp/platform/internal/eventbus"
	"github.com/adrianozp/platform/internal/httpserver/wait"
	"github.com/adrianozp/platform/internal/sweeper"
	"github.com/adrianozp/platform/pkg/approval"
	"go.uber.org/fx"
)

func Approvals() fx.Option {
	return fx.Options(
		fx.Provide(
			approvalsMysql.NewRequestRepository, discord.NewNotifier, submit.NewSubmitUsecase, requests.NewRequestsUsecase,
			approvalsConfig.NewExecutor, approvalsDeploy.NewExecutor, approvalsQueue.NewExecutor, executors,
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
		func(u requests.RequestsUsecase) requestsHandler.Requests { return u },
		func(u requests.RequestsUsecase) wait.Requests { return u },
	)
}

func submitInjections() fx.Option {
	return fx.Provide(
		func(u submit.SubmitUsecase) configs.Requests { return u },
		func(u submit.SubmitUsecase) deployments.Requests { return u },
		func(u submit.SubmitUsecase) queues.Requests { return u },
	)
}

func executors(c approvalsConfig.Executor, d approvalsDeploy.Executor, q approvalsQueue.Executor) requests.Executors {
	return requests.Executors{
		approval.KindConfigRevise: c, approval.KindSecretSet: c, approval.KindSecretUnset: c,
		approval.KindDeploy: d, approval.KindPromote: d, approval.KindRollback: d, approval.KindConfigApply: d, approval.KindRemove: d,
		approval.KindQueueCreate: q, approval.KindQueueUpdate: q, approval.KindQueueDelete: q,
	}
}

func startApprovalsSweeper(lc fx.Lifecycle, u requests.RequestsUsecase) {
	sweeper.Start(lc, "approvals", u)
}
