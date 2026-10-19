package web

import "github.com/klzn-labs/kalzone/pkg/approval"

type ShipLine struct {
	Title      string
	Type       string
	SHA        string
	PullNumber int
	PullURL    string
	Commits    int
}

func shipLines(commits []approval.ShipCommit) []ShipLine {
	lines, byPull := []ShipLine{}, map[int]int{}
	for _, c := range commits {
		if at, seen := byPull[c.PullNumber]; seen && c.PullNumber != 0 {
			lines[at].Commits++
			continue
		}
		byPull[c.PullNumber] = len(lines)
		lines = append(lines, lineOf(c))
	}
	return lines
}

func lineOf(c approval.ShipCommit) ShipLine {
	line := ShipLine{Title: c.Title, Type: c.Type, SHA: c.SHA, PullNumber: c.PullNumber, PullURL: c.PullURL, Commits: 1}
	if c.PullTitle != "" {
		line.Title = c.PullTitle
	}
	return line
}
