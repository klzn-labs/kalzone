package entities

import (
	"regexp"
	"strings"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const (
	UsernamePrefix    = "kz_"
	maxUsernameLength = 32
	minMaxConnections = 1
	maxMaxConnections = 30
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,19}$`)

var reservedSchemas = map[string]bool{
	"mysql": true, "sys": true, "information_schema": true, "performance_schema": true, "platform": true,
}

func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return apperror.Invalid("database name must be 2-20 lowercase letters, digits or dashes, starting with a letter")
	}
	return nil
}

func SchemaFor(name, environment string, protected bool) string {
	base := strings.ReplaceAll(name, "-", "_")
	if protected {
		return base
	}
	return base + "_" + strings.ReplaceAll(environment, "-", "_")
}

func UsernameFor(schema string) (string, error) {
	user := UsernamePrefix + schema
	if len(user) > maxUsernameLength {
		return "", apperror.Invalid("database user name would exceed 32 characters, use a shorter database name").
			WithHint("shorten the database name so that kz_<schema> fits in 32 characters")
	}
	return user, nil
}

func ValidateMaxConnections(n int) error {
	if n < minMaxConnections || n > maxMaxConnections {
		return apperror.Invalid("max connections must be between 1 and 30")
	}
	return nil
}

func Reserved(schema string) bool {
	return reservedSchemas[schema]
}
