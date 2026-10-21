package web

import (
	"encoding/json"
	"time"

	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const (
	InProdLive    = "live"
	InProdWas     = "was"
	InProdWaiting = "waiting"
	InProdNever   = "never"
	statsWindow   = 14 * 24 * time.Hour
)

type DeployStats struct {
	Total  int
	Failed int
}

func (st ServiceStatus) Broken() bool {
	return len(st.Problems) > 0
}

func (st ServiceStatus) Latest() *release.Release {
	if len(st.Releases) == 0 {
		return nil
	}
	return &st.Releases[0]
}

func (st ServiceStatus) Building() bool {
	latest := st.Latest()
	return latest != nil && !latest.Final()
}

func (st ServiceStatus) Live() *deploy.Deployment {
	return st.firstDeploy(func(d deploy.Deployment) bool { return d.Status == deploy.StatusSucceeded })
}

func (st ServiceStatus) Running() *deploy.Deployment {
	return st.firstDeploy(deploy.Deployment.Active)
}

func (st ServiceStatus) lastFinished() *deploy.Deployment {
	return st.firstDeploy(func(d deploy.Deployment) bool {
		return d.Status == deploy.StatusSucceeded || d.Status == deploy.StatusFailed
	})
}

func (st ServiceStatus) firstDeploy(keep func(deploy.Deployment) bool) *deploy.Deployment {
	for i := range st.Deploys {
		if keep(st.Deploys[i]) {
			return &st.Deploys[i]
		}
	}
	return nil
}

func (st ServiceStatus) InProd(r release.Release) string {
	if live := st.Live(); live != nil && live.ReleaseNumber == r.Number {
		return InProdLive
	}
	if st.Waiting != nil && st.waitsFor(r) {
		return InProdWaiting
	}
	for _, d := range st.Deploys {
		if d.Status == deploy.StatusSucceeded && d.ReleaseNumber == r.Number {
			return InProdWas
		}
	}
	return InProdNever
}

func (st ServiceStatus) waitsFor(r release.Release) bool {
	ships := st.Waiting.Kind == approval.KindDeploy || st.Waiting.Kind == approval.KindPromote
	var proposal struct {
		ReleaseID string `json:"release_id"`
	}
	_ = json.Unmarshal(st.Waiting.Payload, &proposal)
	return ships && proposal.ReleaseID != "" && proposal.ReleaseID == r.ID
}

func (st ServiceStatus) FinishedDeploys(limit int) []deploy.Deployment {
	shown := []deploy.Deployment{}
	for _, d := range st.Deploys {
		if d.Status != deploy.StatusCancelled && len(shown) < limit {
			shown = append(shown, d)
		}
	}
	return shown
}

func (st ServiceStatus) Stats() DeployStats {
	since := clock.Now().Add(-statsWindow)
	stats := DeployStats{}
	for _, d := range st.Deploys {
		if d.FinishedAt == nil || d.FinishedAt.Before(since) || d.Status == deploy.StatusCancelled {
			continue
		}
		stats.Total++
		if d.Status == deploy.StatusFailed {
			stats.Failed++
		}
	}
	return stats
}
