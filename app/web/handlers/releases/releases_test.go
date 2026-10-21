package releases

import (
	"testing"

	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/stretchr/testify/require"
)

func rowsNumbered(n int) []Row {
	out := make([]Row, n)
	for i := range out {
		out[i] = Row{Release: release.Release{Number: n - i}}
	}
	return out
}

func Test_Data_ShowsTheFiveNewestReleasesFirst(t *testing.T) {
	d := data{Rows: rowsNumbered(8)}

	require.Len(t, d.RecentRows(), 5)
	require.Equal(t, 8, d.RecentRows()[0].Release.Number)
	require.Len(t, d.OlderRows(), 3)
	require.Equal(t, 3, d.OlderRows()[0].Release.Number)
}

func Test_Data_HasNoOlderReleasesWhenFiveOrFewer(t *testing.T) {
	d := data{Rows: rowsNumbered(5)}

	require.Len(t, d.RecentRows(), 5)
	require.Empty(t, d.OlderRows())
}
