package configs

import (
	"context"
	"fmt"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
)

func (u ConfigUsecase) CurrentNumber(ctx context.Context, serviceSlug, environment string) (int, error) {
	rev, err := u.Current(ctx, serviceSlug, environment)
	return rev.Number, err
}

func (u ConfigUsecase) Resolve(ctx context.Context, serviceID, environment string, number int) (entities.Resolved, error) {
	rev, err := u.revision(ctx, serviceID, environment, number)
	if err != nil {
		return entities.Resolved{}, err
	}
	secrets, err := u.open(ctx, rev)
	return entities.Resolved{Revision: rev, Secrets: secrets}, err
}

func (u ConfigUsecase) revision(ctx context.Context, serviceID, environment string, number int) (entities.Revision, error) {
	if number == 0 {
		return entities.Initial(serviceID, environment), nil
	}
	return u.store.ByNumber(ctx, serviceID, environment, number)
}

func (u ConfigUsecase) open(ctx context.Context, rev entities.Revision) (map[string]string, error) {
	ids := make([]string, 0, len(rev.Secrets))
	for _, id := range rev.Secrets {
		ids = append(ids, id)
	}
	versions, err := u.store.SecretVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	secrets := make(map[string]string, len(versions))
	for _, v := range versions {
		plaintext, err := u.sealer.Open(v.Ciphertext, v.Nonce, AdditionalData(v.ServiceID, v.Environment, v.Name), v.KeyID)
		if err != nil {
			return nil, fmt.Errorf("secret %s could not be opened: %w", v.Name, err)
		}
		secrets[v.Name] = string(plaintext)
	}
	return secrets, nil
}
