package accounts

import (
	"context"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

type SetupUnfinishedError struct {
	Err error
}

func (e SetupUnfinishedError) Error() string {
	return "the owner was created but setup did not finish: " + e.Err.Error()
}

func (e SetupUnfinishedError) Unwrap() error {
	return e.Err
}

func (u SetupUsecase) LinkKind(ctx context.Context, token string) (entities.LinkKind, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return "", err
	}
	link, err := u.links.FindByHash(ctx, entities.HashToken(token))
	if err != nil {
		return "", err
	}
	if !link.Live(clock.Now().UTC()) {
		return "", errLinkGone
	}
	return link.Kind, nil
}
