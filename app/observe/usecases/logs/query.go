package logs

import (
	"strconv"
	"time"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const (
	defaultSince = 15 * time.Minute
	longestSince = 24 * time.Hour
	defaultLimit = 100
	largestLimit = 1000
)

var (
	errSince  = apperror.Invalid("since must be a duration above 0 and at most 24h, such as 15m or 2h").WithField("since")
	errLimit  = apperror.Invalid("limit must be a whole number from 1 to 1000").WithField("limit")
	errColour = apperror.Invalid("colour must be blue or green").WithField("colour").WithHint("leave colour out to read both")
	colours   = map[string]bool{"": true, "blue": true, "green": true}
)

type Query struct {
	Since    time.Duration
	Limit    int
	Contains string
	Colour   string
	Errors   bool
}

func ParseQuery(since, limit, contains, colour string) (Query, error) {
	q := Query{Since: defaultSince, Limit: defaultLimit, Contains: contains, Colour: colour}
	var err error
	if since != "" {
		q.Since, err = time.ParseDuration(since)
	}
	if err != nil || q.Since <= 0 || q.Since > longestSince {
		return Query{}, errSince
	}
	if limit != "" {
		q.Limit, err = strconv.Atoi(limit)
	}
	if err != nil || q.Limit < 1 || q.Limit > largestLimit {
		return Query{}, errLimit
	}
	if !colours[colour] {
		return Query{}, errColour
	}
	return q, nil
}
