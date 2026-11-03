package accounts

import (
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
)

var errGitHubMode = apperror.New(apperror.CodeGitHubMode, "this Kalzone signs people in with GitHub").
	WithHint("this Kalzone signs people in with GitHub; add them to the team after they sign in")

type Mode struct {
	GitHub bool
}

func NewMode(cfg config.Config) Mode {
	return Mode{GitHub: cfg.GitHub.ClientID != "" && cfg.GitHub.ClientSecret != ""}
}

func (m Mode) RequirePasswords() error {
	if m.GitHub {
		return errGitHubMode
	}
	return nil
}
