package probe

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ping-scanner/internal/tlstest"
)

func TestScan(t *testing.T) {
	tests := []struct {
		name     string
		statuses []int
		want     []Outcome
	}{
		{
			name:     "no targets",
			statuses: []int{},
			want:     []Outcome{},
		},
		{
			name:     "one up",
			statuses: []int{http.StatusOK},
			want:     []Outcome{Up},
		},
		{
			name:     "one down",
			statuses: []int{http.StatusInternalServerError},
			want:     []Outcome{Down},
		},
		{
			name:     "all up",
			statuses: []int{http.StatusOK, http.StatusNotFound},
			want:     []Outcome{Up, Up},
		},
		{
			name:     "down after up",
			statuses: []int{http.StatusOK, http.StatusInternalServerError},
			want:     []Outcome{Up, Down},
		},
		{
			name:     "up after down",
			statuses: []int{http.StatusInternalServerError, http.StatusNotFound},
			want:     []Outcome{Down, Up},
		},
		{
			name:     "all down",
			statuses: []int{http.StatusInternalServerError, http.StatusInternalServerError},
			want:     []Outcome{Down, Down},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			targets := make([]Target, len(test.statuses))
			for i, status := range test.statuses {
				targets[i] = startServer(t, func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
				})
			}

			got := Scan(targets, false, 4)

			outcomes := make([]Outcome, len(got))
			for i, result := range got {
				outcomes[i] = result.Outcome
			}
			assert.Equal(t, test.want, outcomes)
		})
	}
}

func TestScan_limitsConcurrency(t *testing.T) {
	const (
		n     = 6
		limit = 2
	)
	var current, maxSeen atomic.Int32
	release := make(chan struct{})
	statuses := []int{200, 201, 202, 204, 205, 206}
	targets := make([]Target, n)
	for i, status := range statuses {
		targets[i] = startServer(t, func(w http.ResponseWriter, _ *http.Request) {
			seen := current.Add(1)
			for {
				old := maxSeen.Load()
				if seen <= old || maxSeen.CompareAndSwap(old, seen) {
					break
				}
			}
			<-release
			w.WriteHeader(status)
		})
	}

	done := make(chan []Result, 1)
	go func() {
		done <- Scan(targets, false, limit)
	}()

	require.Eventually(t, func() bool {
		return current.Load() == limit
	}, time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(limit), current.Load())
	assert.Equal(t, int32(limit), maxSeen.Load())
	close(release)

	var got []Result
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scan did not finish")
	}
	require.Len(t, got, n)
	for i, status := range statuses {
		assert.Equal(t, status, got[i].StatusCode)
	}
}

func TestScan_overlaps(t *testing.T) {
	const (
		n     = 4
		delay = 150 * time.Millisecond
	)
	targets := make([]Target, n)
	for i := range targets {
		targets[i] = startServer(t, func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(delay)
			w.WriteHeader(http.StatusOK)
		})
	}

	start := time.Now()
	got := Scan(targets, false, n)
	elapsed := time.Since(start)

	require.Len(t, got, n)
	assert.Less(t, elapsed, 450*time.Millisecond)
}

func TestScan_selfSigned(t *testing.T) {
	srv := tlstest.Server(t)

	got := Scan([]Target{Target(srv.URL)}, false, 1)

	require.Len(t, got, 1)
	assert.Equal(t, Down, got[0].Outcome)
	assert.Equal(t, 0, got[0].StatusCode)
	assert.True(t, strings.HasPrefix(got[0].Reason(), "TLS:"))
}

func TestScan_insecureSelfSigned(t *testing.T) {
	srv := tlstest.Server(t)

	got := Scan([]Target{Target(srv.URL)}, true, 1)

	require.Len(t, got, 1)
	require.NoError(t, got[0].Err)
	assert.Equal(t, Up, got[0].Outcome)
	assert.Equal(t, http.StatusOK, got[0].StatusCode)
}

func TestExit(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    ExitCode
	}{
		{name: "no results", results: []Result{}, want: ExitUsage},
		{name: "one up", results: []Result{{Outcome: Up}}, want: ExitOK},
		{name: "one down", results: []Result{{Outcome: Down}}, want: ExitDown},
		{name: "all up", results: []Result{{Outcome: Up}, {Outcome: Up}}, want: ExitOK},
		{name: "down after up", results: []Result{{Outcome: Up}, {Outcome: Down}}, want: ExitDown},
		{name: "up after down", results: []Result{{Outcome: Down}, {Outcome: Up}}, want: ExitDown},
		{name: "all down", results: []Result{{Outcome: Down}, {Outcome: Down}}, want: ExitDown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Exit(test.results)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    Summary
	}{
		{name: "no results", results: []Result{}, want: Summary{Up: 0, Down: 0}},
		{name: "one Up", results: []Result{{Outcome: Up}}, want: Summary{Up: 1, Down: 0}},
		{name: "one Down", results: []Result{{Outcome: Down}}, want: Summary{Up: 0, Down: 1}},
		{name: "all Up", results: []Result{{Outcome: Up}, {Outcome: Up}}, want: Summary{Up: 2, Down: 0}},
		{name: "all Down", results: []Result{{Outcome: Down}, {Outcome: Down}}, want: Summary{Up: 0, Down: 2}},
		{name: "Down after Up", results: []Result{{Outcome: Up}, {Outcome: Down}}, want: Summary{Up: 1, Down: 1}},
		{name: "Up after Down", results: []Result{{Outcome: Down}, {Outcome: Up}}, want: Summary{Up: 1, Down: 1}},
		{name: "zero Outcome counts as Down", results: []Result{{}}, want: Summary{Up: 0, Down: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Summarize(test.results)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestSummaryString(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    string
	}{
		{name: "no results", summary: Summary{}, want: "0 up, 0 down, 0 total"},
		{name: "one Up", summary: Summary{Up: 1}, want: "1 up, 0 down, 1 total"},
		{name: "one Down", summary: Summary{Down: 1}, want: "0 up, 1 down, 1 total"},
		{name: "all Up", summary: Summary{Up: 2, Down: 0}, want: "2 up, 0 down, 2 total"},
		{name: "all Down", summary: Summary{Up: 0, Down: 2}, want: "0 up, 2 down, 2 total"},
		{name: "Down after Up", summary: Summary{Up: 1, Down: 1}, want: "1 up, 1 down, 2 total"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.summary.String())
		})
	}
}
