package scope

import (
	"fmt"
	"slices"

	"github.com/adrianozp/platform/pkg/apperror"
)

type Scope string

const (
	Read          Scope = "read"
	Logs          Scope = "logs"
	Services      Scope = "services"
	ServicesAdmin Scope = "services:admin"
	Builds        Scope = "builds"
	Config        Scope = "config"
	Secrets       Scope = "secrets"
	Deploy        Scope = "deploy"
	Protected     Scope = "protected"
	TeamsAdmin    Scope = "teams:admin"
	TokensAdmin   Scope = "tokens:admin"
)

var All = []Scope{Read, Logs, Services, ServicesAdmin, Builds, Config, Secrets, Deploy, Protected, TeamsAdmin, TokensAdmin}

func Parse(names []string) ([]Scope, error) {
	result := make([]Scope, 0, len(names))
	for _, name := range names {
		s := Scope(name)
		if !slices.Contains(All, s) {
			return nil, apperror.Invalid(fmt.Sprintf("unknown scope %q", name))
		}
		if !slices.Contains(result, s) {
			result = append(result, s)
		}
	}
	return result, nil
}
