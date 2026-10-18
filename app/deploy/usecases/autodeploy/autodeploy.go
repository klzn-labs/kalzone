package autodeploy

import (
	"context"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type ServiceLookup interface {
	FindServiceByID(ctx context.Context, id string) (entities.ServiceRef, error)
}

type Deployer interface {
	AutoDeploy(ctx context.Context, serviceID, releaseID string) (entities.Deployment, error)
}

type AutoDeployUsecase struct {
	services ServiceLookup
	deployer Deployer
}

func NewAutoDeployUsecase(services ServiceLookup, deployer Deployer) AutoDeployUsecase {
	return AutoDeployUsecase{services: services, deployer: deployer}
}

func (u AutoDeployUsecase) Handle(ctx context.Context, e events.Event) error {
	releaseID, ok := releaseBuiltFromPush(e)
	if !ok {
		return nil
	}
	service, err := u.services.FindServiceByID(ctx, e.TargetID)
	if err != nil || !service.AutoDeployDev {
		return err
	}
	_, err = u.deployer.AutoDeploy(ctx, service.ID, releaseID)
	return err
}

func releaseBuiltFromPush(e events.Event) (string, bool) {
	if e.TargetKind != events.TargetRelease || e.Details["trigger"] != "push" {
		return "", false
	}
	if e.Action != "release.built" && e.Action != "release.shared" {
		return "", false
	}
	id, _ := e.Details["release_id"].(string)
	return id, id != ""
}
