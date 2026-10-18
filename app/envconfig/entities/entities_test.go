package entities_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/stretchr/testify/require"
)

func Test_Next_CopiesMapsSoOlderRevisionsStayUnchanged(t *testing.T) {
	first := entities.Initial("svc", "dev")
	first.Number = 1
	first.Env["A"] = "1"

	second := first.Next()
	second.Env["A"] = "2"
	second.Secrets["S"] = "v1"

	require.Equal(t, 2, second.Number)
	require.Equal(t, "1", first.Env["A"])
	require.Empty(t, first.Secrets)
}

func Test_Validate_Rules(t *testing.T) {
	base := entities.Initial("svc", "dev")
	require.NoError(t, base.Validate())

	broken := map[string]func(r *entities.Revision){
		"lowercase name":   func(r *entities.Revision) { r.Env["db_url"] = "x" },
		"reserved name":    func(r *entities.Revision) { r.Env["PLATFORM_QUEUE_TOKEN"] = "x" },
		"var is a secret":  func(r *entities.Revision) { r.Env["TOKEN"] = "x"; r.Secrets["TOKEN"] = "v1" },
		"value too long":   func(r *entities.Revision) { r.Env["BIG"] = string(make([]byte, entities.MaxValueBytes+1)) },
		"port":             func(r *entities.Revision) { r.Spec.Port = 0 },
		"health path":      func(r *entities.Revision) { r.Spec.HealthPath = "healthz" },
		"host":             func(r *entities.Revision) { r.Spec.Host = "Not A Host" },
		"memory too small": func(r *entities.Revision) { r.Spec.MemoryMB = 16 },
		"memory too big":   func(r *entities.Revision) { r.Spec.MemoryMB = 8192 },
		"cpu":              func(r *entities.Revision) { r.Spec.CPUMillis = 10 },
		"replicas":         func(r *entities.Revision) { r.Spec.Replicas = 6 },
		"alias, no host":   func(r *entities.Revision) { r.Spec.Aliases = []string{"www.kalz.one"} },
		"bad alias":        func(r *entities.Revision) { r.Spec.Host, r.Spec.Aliases = "kalz.one", []string{"Not A Host"} },
		"alias is host":    func(r *entities.Revision) { r.Spec.Host, r.Spec.Aliases = "kalz.one", []string{"kalz.one"} },
		"too many aliases": func(r *entities.Revision) { r.Spec.Host, r.Spec.Aliases = "kalz.one", make([]string, 6) },
		"internal, host":   func(r *entities.Revision) { r.Spec.Host, r.Spec.Internal = "kalz.one", true },
	}
	for name, breakIt := range broken {
		r := entities.Initial("svc", "dev")
		breakIt(&r)
		require.Error(t, r.Validate(), name)
	}
}

func Test_HostAllowed(t *testing.T) {
	domains := []string{"kalz.one"}

	require.True(t, entities.HostAllowed("api.kalz.one", domains))
	require.True(t, entities.HostAllowed("kalz.one", domains))
	require.False(t, entities.HostAllowed("evilkalz.one", domains))
	require.False(t, entities.HostAllowed("api.example.com", domains))
}

func Test_Diff_HidesSecretValues(t *testing.T) {
	before := entities.Initial("svc", "dev")
	before.Env["KEEP"], before.Env["DROP"], before.Secrets["S"] = "1", "x", "v1"
	after := before.Next()
	after.Env["KEEP"], after.Env["NEW"], after.Secrets["S"] = "2", "y", "v2"
	delete(after.Env, "DROP")
	after.Spec.Replicas = 2

	changes := entities.Diff(before, after)

	require.Equal(t, []entities.Change{
		{Section: "spec", Key: "replicas", Kind: entities.Changed, Before: "1", After: "2"},
		{Section: "env", Key: "DROP", Kind: entities.Removed, Before: "x"},
		{Section: "env", Key: "KEEP", Kind: entities.Changed, Before: "1", After: "2"},
		{Section: "env", Key: "NEW", Kind: entities.Added, After: "y"},
		{Section: "secret", Key: "S", Kind: entities.Changed},
	}, changes)
	require.Equal(t, []string{"spec.replicas", "env.DROP", "env.KEEP", "env.NEW", "secret.S"}, entities.ChangedKeys(changes))
}
