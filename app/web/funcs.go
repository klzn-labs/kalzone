package web

import (
	"errors"
	"fmt"
	"hash/fnv"
	"html/template"
	"strings"
	"time"

	"github.com/adrianozp/platform/pkg/clock"
)

func funcs() template.FuncMap {
	return template.FuncMap{
		"icon":     icon,
		"ago":      Ago,
		"date":     func(t time.Time) string { return t.UTC().Format("Jan 2, 2006 15:04 UTC") },
		"iso":      func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
		"initials": Initials,
		"hue":      Hue,
		"add":      func(a, b int) int { return a + b },
		"dict":     dict,
		"upper":    strings.ToUpper,
	}
}

func Ago(t time.Time) string {
	d := clock.Now().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	case d < 30*24*time.Hour:
		return plural(int(d.Hours()/24), "day") + " ago"
	}
	return t.UTC().Format("Jan 2, 2006")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func Initials(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '-' || r == '_' })
	result := ""
	for _, w := range words {
		if len(result) < 2 {
			result += strings.ToUpper(w[:1])
		}
	}
	if result == "" {
		return "?"
	}
	return result
}

func Hue(key string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % 6)
}

func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("dict needs key/value pairs")
	}
	result := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}
		result[key] = values[i+1]
	}
	return result, nil
}
