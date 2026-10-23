package cronspec

import (
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/robfig/cron/v3"
)

const hint = "use five fields in UTC, minute hour day-of-month month day-of-week, such as */5 * * * *, or @hourly or @daily"

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

var refusedPrefixes = []string{"@every", "CRON_TZ=", "TZ="}

type Schedule struct {
	spec     string
	schedule cron.Schedule
}

func Parse(spec string) (Schedule, error) {
	spec = strings.TrimSpace(spec)
	if refused(spec) {
		return Schedule{}, invalid()
	}
	parsed, err := parser.Parse(spec)
	if err != nil {
		return Schedule{}, invalid()
	}
	return Schedule{spec: spec, schedule: parsed}, nil
}

func refused(spec string) bool {
	for _, prefix := range refusedPrefixes {
		if strings.HasPrefix(spec, prefix) {
			return true
		}
	}
	return false
}

func invalid() error {
	return apperror.Invalid("schedule is not a valid cron expression").WithField("schedule").WithHint(hint)
}

func (s Schedule) Next(after time.Time) time.Time {
	return s.schedule.Next(after.UTC()).UTC()
}

func (s Schedule) String() string {
	return s.spec
}
