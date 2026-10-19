package releases

import (
	"fmt"

	release "github.com/klzn-labs/kalzone/app/release/entities"
)

type PullGroup struct {
	Pull    *release.PullRef
	Commits []release.Commit
}

func pullGroups(commits []release.Commit) []PullGroup {
	groups, byNumber := []PullGroup{}, map[int]int{}
	for _, c := range commits {
		if c.PullRequest == nil {
			groups = append(groups, PullGroup{Commits: []release.Commit{c}})
			continue
		}
		if at, seen := byNumber[c.PullRequest.Number]; seen {
			groups[at].Commits = append(groups[at].Commits, c)
			continue
		}
		byNumber[c.PullRequest.Number] = len(groups)
		groups = append(groups, PullGroup{Pull: c.PullRequest, Commits: []release.Commit{c}})
	}
	return groups
}

func changesLabel(c release.Changelog, numbers map[string]int) string {
	switch c.State {
	case release.ChangelogAhead, release.ChangelogDiverged:
		return commitCount(len(c.Commits)) + " since " + baseName(c.BaseReleaseID, numbers)
	case release.ChangelogFirst:
		return "first release"
	case release.ChangelogPending:
		return "reading GitHub…"
	case release.ChangelogFailed:
		return "changelog failed"
	}
	return ""
}

func commitCount(n int) string {
	if n == 1 {
		return "1 commit"
	}
	return fmt.Sprintf("%d commits", n)
}

func baseName(id string, numbers map[string]int) string {
	if n, ok := numbers[id]; ok {
		return fmt.Sprintf("r%d", n)
	}
	return "the last release"
}
