package migrator

import (
	"errors"

	"go.uber.org/fx"
)

func MigrateOnStart(lc fx.Lifecycle, m *Migrator) {
	lc.Append(fx.StartHook(func() error {
		return errors.Join(m.Up(), m.Close())
	}))
}
