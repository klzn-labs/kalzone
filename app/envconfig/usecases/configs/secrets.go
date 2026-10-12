package configs

import (
	"context"
	"fmt"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

var errSecretNotFound = apperror.NotFound("secret not found")

func (u ConfigUsecase) SetSecret(ctx context.Context, a actor.Actor, serviceSlug, environment, name, value string) (entities.Revision, error) {
	if err := validateSecret(name, value); err != nil {
		return entities.Revision{}, err
	}
	e, err := u.editable(ctx, a, authz.EditSecrets, serviceSlug, environment)
	if err != nil {
		return entities.Revision{}, err
	}
	version, err := u.seal(ctx, a, e.target, name, value)
	if err != nil {
		return entities.Revision{}, err
	}
	cs := entities.ChangeSet{SetSecrets: map[string]string{name: version.ID}, Message: "set secret " + name}
	return u.change(ctx, a, e, approval.KindSecretSet, cs)
}

func (u ConfigUsecase) UnsetSecret(ctx context.Context, a actor.Actor, serviceSlug, environment, name string) (entities.Revision, error) {
	e, err := u.editable(ctx, a, authz.EditSecrets, serviceSlug, environment)
	if err != nil {
		return entities.Revision{}, err
	}
	if _, exists := e.current.Secrets[name]; !exists {
		return entities.Revision{}, errSecretNotFound
	}
	cs := entities.ChangeSet{UnsetSecrets: []string{name}, Message: "removed secret " + name}
	return u.change(ctx, a, e, approval.KindSecretUnset, cs)
}

func validateSecret(name, value string) error {
	if err := entities.ValidateName(name); err != nil {
		return err
	}
	if value == "" || len(value) > entities.MaxSecretBytes {
		return apperror.Invalid(fmt.Sprintf("a secret value must be 1-%d bytes", entities.MaxSecretBytes))
	}
	return nil
}

func (u ConfigUsecase) seal(ctx context.Context, a actor.Actor, t target, name, value string) (entities.SecretVersion, error) {
	ciphertext, nonce, keyID, err := u.sealer.Seal([]byte(value), AdditionalData(t.service.ID, t.environment, name))
	if err != nil {
		return entities.SecretVersion{}, err
	}
	version := entities.SecretVersion{
		ID: uuid.NewUUID(), ServiceID: t.service.ID, Environment: t.environment, Name: name,
		Ciphertext: ciphertext, Nonce: nonce, KeyID: keyID, CreatedBy: a.UserID, CreatedAt: clock.Now().UTC(),
	}
	return version, u.store.InsertSecretVersion(ctx, version)
}

func AdditionalData(serviceID, environment, name string) []byte {
	return []byte(serviceID + "|" + environment + "|" + name)
}

func (u ConfigUsecase) Secrets(ctx context.Context, serviceSlug, environment string) ([]entities.SecretInfo, error) {
	current, err := u.Current(ctx, serviceSlug, environment)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(current.Secrets))
	for _, id := range current.Secrets {
		ids = append(ids, id)
	}
	versions, err := u.store.SecretVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	infos := make([]entities.SecretInfo, 0, len(versions))
	for _, v := range versions {
		infos = append(infos, entities.SecretInfo{Name: v.Name, VersionID: v.ID, SetBy: v.CreatedBy, SetAt: v.CreatedAt})
	}
	return infos, nil
}
