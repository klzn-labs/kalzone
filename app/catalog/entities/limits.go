package entities

import (
	"fmt"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const (
	maxResponseMS   = 60000
	maxCPUPercent   = 100
	maxDBCPUPercent = 400
)

func (l Limits) Validate() error {
	for _, bound := range []struct {
		value, max  int
		field, name string
	}{
		{l.ResponseMS, maxResponseMS, "limits.response_ms", "response time limit"},
		{l.CPUPercent, maxCPUPercent, "limits.cpu_percent", "CPU limit"},
		{l.DBCPUPercent, maxDBCPUPercent, "limits.db_cpu_percent", "database CPU limit"},
	} {
		if bound.value < 0 || bound.value > bound.max {
			return apperror.Invalid(fmt.Sprintf("%s must be from 1 to %d, or 0 for the platform default", bound.name, bound.max)).WithField(bound.field)
		}
	}
	return nil
}

func numberOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func (c ServiceChanges) applyLimits(l Limits) Limits {
	return Limits{
		ResponseMS:   numberOr(c.ResponseLimitMS, l.ResponseMS),
		CPUPercent:   numberOr(c.CPULimitPercent, l.CPUPercent),
		DBCPUPercent: numberOr(c.DBCPULimitPercent, l.DBCPUPercent),
	}
}
