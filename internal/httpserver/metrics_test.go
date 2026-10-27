package httpserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_RequestLabels_FoldEveryUnmatchedMethodAndPathIntoOneSeries(t *testing.T) {
	method, route := requestLabels("PROPFIND", "")

	require.Equal(t, "ANY", method)
	require.Equal(t, "unmatched", route)
}

func Test_RequestLabels_KeepAMatchedRoutesMethod(t *testing.T) {
	method, route := requestLabels("POST", "/api/v1/queues")

	require.Equal(t, "POST", method)
	require.Equal(t, "/api/v1/queues", route)
}
