package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeLinks struct{ resetFor string }

func (f *fakeLinks) NewSetupLink(context.Context) (string, error) {
	return "https://kalzone.test/join/setup", nil
}

func (f *fakeLinks) NewResetLinkFor(_ context.Context, login string) (string, error) {
	f.resetFor = login
	return "https://kalzone.test/join/reset", nil
}

func Test_SetupLinkArgsChooseTheLink(t *testing.T) {
	var stderr bytes.Buffer
	links := &fakeLinks{}

	none, err := parseSetupLinkArgs(nil, &stderr)
	require.NoError(t, err)
	setup, err := makeLink(links, none)
	require.NoError(t, err)
	reset, err := parseSetupLinkArgs([]string{"--reset", "ana"}, &stderr)
	require.NoError(t, err)
	resetURL, err := makeLink(links, reset)
	require.NoError(t, err)

	require.Equal(t, "https://kalzone.test/join/setup", setup)
	require.Equal(t, "https://kalzone.test/join/reset", resetURL)
	require.Equal(t, "ana", links.resetFor)
}

func Test_SetupLinkRefusesStrayArguments(t *testing.T) {
	var stderr bytes.Buffer

	_, unknown := parseSetupLinkArgs([]string{"--owner", "ana"}, &stderr)
	_, stray := parseSetupLinkArgs([]string{"ana"}, &stderr)

	require.Error(t, unknown)
	require.Error(t, stray)
	require.Contains(t, stderr.String(), `unexpected argument "ana"`)
}
