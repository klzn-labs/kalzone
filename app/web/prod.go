package web

import catalog "github.com/klzn-labs/kalzone/app/catalog/entities"

func ProdEnvironment(envs []catalog.Environment) (catalog.Environment, bool) {
	for _, env := range envs {
		if env.Protected {
			return env, true
		}
	}
	return catalog.Environment{}, false
}
