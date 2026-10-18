package slug

import (
	"regexp"
	"strings"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var pattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,38}[a-z0-9]$`)

func Validate(kind, value string) error {
	if !pattern.MatchString(value) || strings.Contains(value, "--") {
		return apperror.Invalid(kind + " must be 3-40 lowercase letters, digits or single dashes, starting with a letter")
	}
	return nil
}

func ValidateName(kind, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > 120 {
		return apperror.Invalid(kind + " name must be 1-120 characters")
	}
	return nil
}
