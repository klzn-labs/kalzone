package deployments

import (
	"context"
	"errors"
	"fmt"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
)

const MaxTargets = 10

var (
	errTargetCount = apperror.Invalid(fmt.Sprintf("send 1 to %d targets", MaxTargets)).WithField("targets").
			WithHint(fmt.Sprintf("split a larger batch into batches of %d", MaxTargets))
	errIncompleteTarget = apperror.Invalid("each target needs a service and an environment").
				WithHint(`send targets as [{"service": "api", "environment": "dev"}]`)
	errRepeatedTarget = apperror.Invalid("a service and environment appear twice in targets").
				WithHint("send each service and environment once")
	errBatchNeedsApproval = apperror.New(apperror.CodeApprovalRequired, "a batch cannot include a protected environment that needs approval").
				WithHint("send the targets on protected environments one at a time; each becomes a change request")
)

type Target struct {
	Service     string
	Environment string
}

type Batch struct {
	Targets  []Target
	Release  string
	Strategy string
}

type targetFailure struct {
	Index   int           `json:"index"`
	Code    apperror.Code `json:"code"`
	Message string        `json:"message"`
	err     error
}

type planner func(Target) (plan, error)

func (u DeploymentsUsecase) DeployAll(ctx context.Context, a actor.Actor, b Batch) ([]entities.Deployment, error) {
	strategy, err := entities.ParseStrategy(b.Strategy)
	if err != nil {
		return nil, err
	}
	plans, err := u.planAll(ctx, b.Targets, func(t Target) (plan, error) {
		return u.deployPlan(ctx, a, t, b.Release, strategy)
	})
	if err != nil {
		return nil, err
	}
	return u.scheduleAll(ctx, plans)
}

func (u DeploymentsUsecase) PromoteAll(ctx context.Context, a actor.Actor, targets []Target, from string) ([]entities.Deployment, error) {
	plans, err := u.planAll(ctx, targets, func(t Target) (plan, error) {
		return u.promotePlan(ctx, a, t, from)
	})
	if err != nil {
		return nil, err
	}
	return u.scheduleAll(ctx, plans)
}

func (u DeploymentsUsecase) planAll(ctx context.Context, targets []Target, planOne planner) ([]plan, error) {
	if err := checkTargets(targets); err != nil {
		return nil, err
	}
	plans, failures := []plan{}, []targetFailure{}
	for i, t := range targets {
		p, err := u.batchable(ctx, planOne, t)
		if err != nil {
			failures = append(failures, failureOf(i, err))
			continue
		}
		plans = append(plans, p)
	}
	return plans, refusal(failures)
}

func (u DeploymentsUsecase) batchable(ctx context.Context, planOne planner, t Target) (plan, error) {
	p, err := planOne(t)
	if err != nil {
		return plan{}, err
	}
	if p.gate == authz.NeedsApproval {
		return plan{}, errBatchNeedsApproval
	}
	_, err = u.idle(ctx, p.service.ID, p.env)
	return p, err
}

func (u DeploymentsUsecase) scheduleAll(ctx context.Context, plans []plan) ([]entities.Deployment, error) {
	pending := make([]entities.Deployment, 0, len(plans))
	for _, p := range plans {
		pending = append(pending, newDeployment(p))
	}
	created, err := u.store.ScheduleAll(ctx, pending)
	if err != nil {
		return nil, err
	}
	for i, p := range plans {
		u.announce(ctx, p, created[i])
	}
	return created, nil
}

func checkTargets(targets []Target) error {
	if len(targets) == 0 || len(targets) > MaxTargets {
		return errTargetCount
	}
	seen := map[Target]bool{}
	for i, t := range targets {
		if t.Service == "" || t.Environment == "" {
			return errIncompleteTarget.WithField(targetField(i))
		}
		if seen[t] {
			return errRepeatedTarget.WithField(targetField(i))
		}
		seen[t] = true
	}
	return nil
}

func failureOf(index int, err error) targetFailure {
	failure := targetFailure{Index: index, Code: apperror.CodeOf(err), Message: "internal error", err: err}
	if failure.Code != apperror.CodeInternal {
		failure.Message = err.Error()
	}
	return failure
}

func refusal(failures []targetFailure) error {
	if len(failures) == 0 {
		return nil
	}
	lead := leading(failures)
	var appErr *apperror.AppError
	if !errors.As(lead.err, &appErr) {
		return lead.err
	}
	return appErr.WithField(targetField(lead.Index)).WithDetails(map[string]any{"targets": failures})
}

func leading(failures []targetFailure) targetFailure {
	for _, f := range failures {
		if f.Code == apperror.CodeApprovalRequired {
			return f
		}
	}
	return failures[0]
}

func targetField(index int) string {
	return fmt.Sprintf("targets[%d]", index)
}
