package benchparser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeMetric(t *testing.T) {
	t.Parallel()

	require.Equal(t, MetricSecPerOp, NormalizeMetric("ns/op"))
	require.Equal(t, MetricSecPerOp, NormalizeMetric("time/op"))
	require.Equal(t, MetricBytesPerOp, NormalizeMetric("bytes/op"))
	require.Equal(t, "MB/s", NormalizeMetric("MB/s"))
}

func TestNormalizeRawMetric(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: MetricNanosecondsPerOp, want: MetricSecPerOp, ok: true},
		{raw: MetricBytesPerOp, want: MetricBytesPerOp, ok: true},
		{raw: MetricAllocsPerOp, want: MetricAllocsPerOp, ok: true},
		{raw: "MB/s", want: "", ok: false},
	} {
		got, ok := NormalizeRawMetric(test.raw)
		require.Equal(t, test.ok, ok)
		require.Equal(t, test.want, got)
	}
}
