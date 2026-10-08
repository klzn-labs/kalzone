package entities

import (
	"sort"
	"strconv"
	"strings"
)

type ChangeKind string

const (
	Added   ChangeKind = "added"
	Removed ChangeKind = "removed"
	Changed ChangeKind = "changed"
)

type Change struct {
	Section string
	Key     string
	Kind    ChangeKind
	Before  string
	After   string
}

func Diff(before, after Revision) []Change {
	changes := diffSpec(before.Spec, after.Spec)
	changes = append(changes, diffMap("env", before.Env, after.Env, true)...)
	return append(changes, diffMap("secret", before.Secrets, after.Secrets, false)...)
}

func diffSpec(before, after Spec) []Change {
	pairs := []struct{ key, before, after string }{
		{"port", strconv.Itoa(before.Port), strconv.Itoa(after.Port)},
		{"health_path", before.HealthPath, after.HealthPath},
		{"host", before.Host, after.Host},
		{"aliases", strings.Join(before.Aliases, ","), strings.Join(after.Aliases, ",")},
		{"internal", strconv.FormatBool(before.Internal), strconv.FormatBool(after.Internal)},
		{"command", before.Command, after.Command},
		{"memory_mb", strconv.Itoa(before.MemoryMB), strconv.Itoa(after.MemoryMB)},
		{"cpu_millis", strconv.Itoa(before.CPUMillis), strconv.Itoa(after.CPUMillis)},
		{"replicas", strconv.Itoa(before.Replicas), strconv.Itoa(after.Replicas)},
	}
	changes := []Change{}
	for _, p := range pairs {
		if p.before != p.after {
			changes = append(changes, Change{Section: "spec", Key: p.key, Kind: Changed, Before: p.before, After: p.after})
		}
	}
	return changes
}

func diffMap(section string, before, after map[string]string, showValues bool) []Change {
	changes := []Change{}
	for _, key := range unionKeys(before, after) {
		old, hadOld := before[key]
		updated, hasNew := after[key]
		if kind, differs := changeKind(hadOld, hasNew, old != updated); differs {
			changes = append(changes, visible(Change{Section: section, Key: key, Kind: kind, Before: old, After: updated}, showValues))
		}
	}
	return changes
}

func changeKind(hadOld, hasNew, valueDiffers bool) (ChangeKind, bool) {
	switch {
	case !hadOld && hasNew:
		return Added, true
	case hadOld && !hasNew:
		return Removed, true
	case valueDiffers:
		return Changed, true
	}
	return "", false
}

func visible(c Change, showValues bool) Change {
	if !showValues {
		c.Before, c.After = "", ""
	}
	return c
}

func unionKeys(a, b map[string]string) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func ChangedKeys(changes []Change) []string {
	keys := make([]string, 0, len(changes))
	for _, c := range changes {
		keys = append(keys, c.Section+"."+c.Key)
	}
	return keys
}
