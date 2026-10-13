package page_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/stretchr/testify/require"
)

func key(n int) string { return strconv.Itoa(n) }

func Test_Parse_DefaultsAndBounds(t *testing.T) {
	defaults, err := page.Parse("", "")
	require.NoError(t, err)
	five, err := page.Parse("5", "")
	require.NoError(t, err)

	require.Equal(t, page.Request{Limit: 20}, defaults)
	require.Equal(t, 5, five.Limit)
	for _, bad := range []string{"0", "101", "ten", "-1"} {
		_, err := page.Parse(bad, "")
		require.ErrorIs(t, err, apperror.Invalid(""), bad)
		require.Equal(t, "limit", err.(*apperror.AppError).Field, bad)
	}
	_, err = page.Parse("0", "")
	require.Equal(t, "limit must be a whole number from 1 to 100", err.Error())
	require.Equal(t, "leave limit out for 20 items, or send 1 to 100", err.(*apperror.AppError).Hint)
}

func Test_Parse_RoundTripsACursorAndRejectsGarbage(t *testing.T) {
	r, err := page.Parse("", page.Encode("42"))
	require.NoError(t, err)
	n, err := r.Number()
	require.NoError(t, err)
	_, garbage := page.Parse("", "%%%")
	notANumber, _ := page.Parse("", page.Encode("abc"))
	_, wrongKind := notANumber.Number()

	require.Equal(t, 42, n)
	require.Equal(t, "cursor", garbage.(*apperror.AppError).Field)
	require.ErrorIs(t, wrongKind, apperror.Invalid(""))
}

func Test_TimeKeyRoundTrips(t *testing.T) {
	at := time.Date(2026, 10, 12, 9, 0, 0, 123000000, time.UTC)
	r, err := page.Parse("", page.Encode(page.TimeKey(at, "e1")))
	require.NoError(t, err)

	got, id, err := r.Time()
	none, _, _ := page.First(5).Time()

	require.NoError(t, err)
	require.True(t, at.Equal(*got))
	require.Equal(t, "e1", id)
	require.Nil(t, none)
}

func Test_Cut_TrimsTheExtraItemIntoACursor(t *testing.T) {
	r := page.First(2)

	short := page.Cut([]int{1}, r, key)
	exact := page.Cut([]int{1, 2}, r, key)
	more := page.Cut([]int{1, 2, 3}, r, key)

	require.Equal(t, page.Page[int]{Items: []int{1}}, short)
	require.Equal(t, page.Page[int]{Items: []int{1, 2}}, exact)
	require.Equal(t, []int{1, 2}, more.Items)
	require.Equal(t, page.Encode("2"), more.Next)
}

func Test_Window_PagesThroughAListInMemory(t *testing.T) {
	all := []int{10, 20, 30, 40, 50}

	first, err := page.Window(all, page.First(2), key)
	require.NoError(t, err)
	after, err := page.Parse("2", first.Next)
	require.NoError(t, err)
	second, err := page.Window(all, after, key)
	require.NoError(t, err)
	_, vanished := page.Window([]int{10, 40, 50}, after, key)

	require.Equal(t, []int{10, 20}, first.Items)
	require.Equal(t, []int{30, 40}, second.Items)
	require.NotEmpty(t, second.Next)
	require.ErrorIs(t, vanished, apperror.Invalid(""))
}

func Test_ALimitBelowOneFallsBackToTheDefault(t *testing.T) {
	all := []int{10, 20, 30}

	cut := page.Cut(all, page.Request{}, key)
	window, err := page.Window(all, page.First(-1), key)

	require.NoError(t, err)
	require.Equal(t, page.Page[int]{Items: all}, cut)
	require.Equal(t, page.Page[int]{Items: all}, window)
	require.Equal(t, page.DefaultLimit+1, page.First(0).Fetch())
}

func Test_TimeKey_KeepsTheStoredMillisecondPrecision(t *testing.T) {
	stored := time.Date(2026, 10, 12, 9, 0, 0, 124000000, time.UTC)

	roundedUp := page.TimeKey(stored.Add(-400*time.Microsecond), "e1")
	roundedDown := page.TimeKey(stored.Add(300*time.Microsecond), "e1")

	require.Equal(t, page.TimeKey(stored, "e1"), roundedUp)
	require.Equal(t, page.TimeKey(stored, "e1"), roundedDown)
	require.Equal(t, "2026-10-12T09:00:00.124Z|e1", roundedUp)
}
