package entities

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/adrianozp/platform/pkg/apperror"
)

const (
	MaxValueBytes  = 4096
	MaxSecretBytes = 8192
	MaxAliases     = 5
)

var (
	namePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)
	hostPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$|^localhost$`)
)

var reserved = map[string]bool{"PLATFORM_QUEUE_URL": true, "PLATFORM_QUEUE_TOKEN": true, "PLATFORM_QUEUE_SIGNING_KEY": true}

func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return apperror.Invalid(fmt.Sprintf("%q is not a valid name: use UPPER_SNAKE_CASE", name))
	}
	if reserved[name] {
		return apperror.Invalid(fmt.Sprintf("%s is set by the platform", name))
	}
	return nil
}

func (r Revision) Validate() error {
	if err := r.Spec.Validate(); err != nil {
		return err
	}
	for name, value := range r.Env {
		if err := validateVariable(r, name, value); err != nil {
			return err
		}
	}
	return nil
}

func validateVariable(r Revision, name, value string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if len(value) > MaxValueBytes {
		return apperror.Invalid(fmt.Sprintf("%s is longer than %d bytes", name, MaxValueBytes))
	}
	if _, clash := r.Secrets[name]; clash {
		return apperror.Invalid(fmt.Sprintf("%s is already a secret", name))
	}
	return nil
}

func (s Spec) Validate() error {
	checks := []struct {
		ok      bool
		message string
	}{
		{s.Port >= 1 && s.Port <= 65535, "port must be between 1 and 65535"},
		{strings.HasPrefix(s.HealthPath, "/") && len(s.HealthPath) <= 255, "health path must start with /"},
		{s.Host == "" || hostPattern.MatchString(s.Host), "host must be a lowercase domain name"},
		{s.MemoryMB >= 32 && s.MemoryMB <= 4096, "memory must be between 32 and 4096 MB"},
		{s.CPUMillis >= 50 && s.CPUMillis <= 4000, "CPU must be between 50 and 4000 millicores"},
		{s.Replicas >= 1 && s.Replicas <= 5, "replicas must be between 1 and 5"},
		{len(s.Command) <= 255 && !strings.ContainsAny(s.Command, "\n\r"), "command must be one line of at most 255 characters"},
	}
	for _, c := range checks {
		if !c.ok {
			return apperror.Invalid(c.message)
		}
	}
	return s.validateRoute()
}

func (s Spec) validateRoute() error {
	switch {
	case s.Internal && s.Host != "":
		return apperror.Invalid("an internal service has no public host")
	case len(s.Aliases) > 0 && s.Host == "":
		return apperror.Invalid("aliases need a host")
	case len(s.Aliases) > MaxAliases:
		return apperror.Invalid(fmt.Sprintf("at most %d aliases", MaxAliases))
	}
	for _, alias := range s.Aliases {
		if alias == s.Host || !hostPattern.MatchString(alias) {
			return apperror.Invalid(fmt.Sprintf("%q is not a valid alias: use a lowercase domain other than the host", alias))
		}
	}
	return nil
}

func (s Spec) Hosts() []string {
	if s.Host == "" {
		return nil
	}
	return append([]string{s.Host}, s.Aliases...)
}

func HostAllowed(host string, domains []string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func (c ChangeSet) ApplyTo(r Revision) Revision {
	if c.Spec != nil {
		r.Spec = *c.Spec
	}
	for name, value := range c.SetEnv {
		r.Env[name] = value
	}
	for _, name := range c.UnsetEnv {
		delete(r.Env, name)
	}
	r.Message = strings.TrimSpace(c.Message)
	return r
}
