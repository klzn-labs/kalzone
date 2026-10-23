package entities

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/cronspec"
)

const (
	MaxPayloadBytes = 64 << 10
	MinTimeout      = time.Second
	MaxTimeout      = 15 * time.Minute
	maxPathLength   = 255
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return apperror.Invalid("cron name must be 2-63 lowercase letters, digits or dashes, starting with a letter").WithField("name")
	}
	return nil
}

func ValidatePath(path string) error {
	if !strings.HasPrefix(path, "/") || len(path) > maxPathLength {
		return apperror.Invalid("path must start with / and be at most 255 characters").WithField("path")
	}
	return nil
}

func NormalizePayload(payload []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if len(trimmed) > MaxPayloadBytes {
		return nil, apperror.Invalid("payload must be at most 64 KiB").WithField("payload")
	}
	if !json.Valid(trimmed) {
		return nil, apperror.Invalid("payload must be valid JSON").WithField("payload")
	}
	return trimmed, nil
}

func ValidateTimeout(timeout time.Duration) error {
	if timeout < MinTimeout || timeout > MaxTimeout {
		return apperror.Invalid("timeout must be between 1s and 15m").WithField("timeout")
	}
	return nil
}

func (c Cron) Validate() error {
	if err := ValidateName(c.Name); err != nil {
		return err
	}
	if _, err := cronspec.Parse(c.Schedule); err != nil {
		return err
	}
	if err := ValidatePath(c.Path); err != nil {
		return err
	}
	return ValidateTimeout(c.Timeout)
}
