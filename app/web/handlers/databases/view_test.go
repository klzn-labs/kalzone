package databases

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/stretchr/testify/require"
)

func Test_Blocker_NamesEachServiceInOneSentenceOnly(t *testing.T) {
	gone := time.Now()
	views := []databases.AttachmentView{
		{Service: "api"},
		{Service: "worker", DetachedAt: &gone, Draining: true},
		{Service: "old", DetachedAt: &gone},
	}

	require.Equal(t, "Detach these first: api. Redeploy or remove: worker.", blocker(views))
}

func Test_Blocker_IsEmptyWhenNothingBlocks(t *testing.T) {
	gone := time.Now()

	require.Empty(t, blocker([]databases.AttachmentView{{Service: "old", DetachedAt: &gone}}))
}

func Test_Blocker_IgnoresDrainingFlagOnAnActiveAttachment(t *testing.T) {
	views := []databases.AttachmentView{{Service: "api", Draining: true}}

	require.Equal(t, "Detach these first: api.", blocker(views))
}
