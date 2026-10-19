package databases_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/stretchr/testify/require"
)

func Test_Access_InjectsTheSixValues(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop")
	password := f.provisioner.created[0].password

	env, secrets, err := f.open().Access(ctx, "s-shop", "dev")

	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"PLATFORM_DB_HOST": "mysql", "PLATFORM_DB_PORT": "3306", "PLATFORM_DB_NAME": "shop_dev",
	}, env)
	require.Equal(t, map[string]string{
		"PLATFORM_DB_USER":     "kz_shop_dev",
		"PLATFORM_DB_PASSWORD": password,
		"PLATFORM_DB_DSN":      "kz_shop_dev:" + password + "@tcp(mysql:3306)/shop_dev?parseTime=true",
	}, secrets)
}

func Test_Access_EmptyWithoutAttachment(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())

	env, secrets, err := f.open().Access(ctx, "s-shop", "dev")

	require.NoError(t, err)
	require.Empty(t, env)
	require.Empty(t, secrets)
	require.NotNil(t, env)
	require.NotNil(t, secrets)
}

func Test_Access_IgnoresDetached(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop")
	_, _ = f.open().Detach(ctx, ana, "dev", "shop", "shop")

	env, secrets, err := f.open().Access(ctx, "s-shop", "dev")

	require.NoError(t, err)
	require.Empty(t, env)
	require.Empty(t, secrets)
}

func Test_Access_RefusesATamperedRow(t *testing.T) {
	f := newFixture(t)
	db := f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop")
	db.ID = "another"
	f.store.databases[f.store.attachments[0].DatabaseID] = db

	_, _, err := f.open().Access(ctx, "s-shop", "dev")

	require.Error(t, err)
}

func Test_Access_PointsAtTheConfiguredEndpoint(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop")
	password := f.provisioner.created[0].password
	u := f.at(policy{}, databases.Endpoint{Host: "127.0.0.1", Port: 3307})

	env, secrets, err := u.Access(ctx, "s-shop", "dev")

	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", env["PLATFORM_DB_HOST"])
	require.Equal(t, "3307", env["PLATFORM_DB_PORT"])
	require.Equal(t, "kz_shop_dev:"+password+"@tcp(127.0.0.1:3307)/shop_dev?parseTime=true", secrets["PLATFORM_DB_DSN"])
	require.Equal(t, databases.Endpoint{Host: "127.0.0.1", Port: 3307}, u.Endpoint())
}
