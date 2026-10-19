package changelogs

import (
	"unicode/utf8"

	"github.com/klzn-labs/kalzone/app/release/entities"
)

func fromComparison(c entities.Comparison) entities.Changelog {
	return entities.Changelog{
		State: stateOf(c.Status), AheadBy: c.AheadBy, BehindBy: c.BehindBy,
		Truncated: c.Truncated, CompareURL: c.URL, Commits: c.Commits,
	}
}

func stateOf(status string) entities.ChangelogState {
	if status == "ahead" || status == "identical" {
		return entities.ChangelogAhead
	}
	return entities.ChangelogDiverged
}

func rebased(c entities.Changelog, t target) entities.Changelog {
	c.ReleaseID, c.BaseReleaseID, c.BaseSHA, c.HeadSHA = t.release.ID, t.base.ID, t.base.CommitSHA, t.release.CommitSHA
	c.Attempts, c.LastError = 0, ""
	return c
}

func retried(previous entities.Changelog, t target, err error) entities.Changelog {
	c := entities.Changelog{
		ReleaseID: t.release.ID, BaseReleaseID: t.base.ID, BaseSHA: t.base.CommitSHA, HeadSHA: t.release.CommitSHA,
		State: entities.ChangelogPending, Attempts: previous.Attempts + 1, LastError: clip(err.Error()), UpdatedAt: now(),
	}
	if c.Attempts >= entities.MaxChangelogAttempts {
		c.State = entities.ChangelogFailed
	}
	return c
}

func clip(s string) string {
	if len(s) <= entities.ErrorLimit {
		return s
	}
	cut := entities.ErrorLimit
	for !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
