package entities

import (
	"slices"
	"strings"
)

func (c ChangeSet) Keys() []string {
	keys := []string{}
	if c.Spec != nil {
		keys = append(keys, "spec")
	}
	keys = prefixed(keys, "env.", unionKeys(c.SetEnv, nil), c.UnsetEnv)
	keys = prefixed(keys, "secret.", unionKeys(c.SetSecrets, nil), c.UnsetSecrets)
	slices.Sort(keys)
	return slices.Compact(keys)
}

func prefixed(keys []string, prefix string, groups ...[]string) []string {
	for _, group := range groups {
		for _, name := range group {
			keys = append(keys, prefix+name)
		}
	}
	return keys
}

func Conflicts(c ChangeSet, base, current Revision) []string {
	moved := map[string]bool{}
	for _, key := range ChangedKeys(Diff(base, current)) {
		moved[unitOf(key)] = true
	}
	return slices.DeleteFunc(c.Keys(), func(key string) bool { return !moved[key] })
}

func unitOf(key string) string {
	if strings.HasPrefix(key, "spec.") {
		return "spec"
	}
	return key
}
