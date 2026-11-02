package probe

import (
	"context"
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

			got := Scan(t.Context(), targets, false, 4, 5*time.Second)

			outcomes := make([]Outcome, len(got.Finished))
			for i, finished := range got.Finished {
				assert.Equal(t, targets[i], finished.Target)
				outcomes[i] = finished.Outcome
			}
			assert.Equal(t, test.want, outcomes)
			assert.False(t, got.interrupted)
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

	done := make(chan ScanResult, 1)
	go func() {
		done <- Scan(t.Context(), targets, false, limit, 5*time.Second)
	}()

	require.Eventually(t, func() bool {
		return current.Load() == limit
	}, time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(limit), current.Load())
	assert.Equal(t, int32(limit), maxSeen.Load())
	close(release)

	var got ScanResult
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scan did not finish")
	}
	require.Len(t, got.Finished, n)
	for i, status := range statuses {
		assert.Equal(t, targets[i], got.Finished[i].Target)
		assert.Equal(t, status, got.Finished[i].StatusCode)
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
	got := Scan(t.Context(), targets, false, n, 5*time.Second)
	elapsed := time.Since(start)

	require.Len(t, got.Finished, n)
	assert.Less(t, elapsed, 450*time.Millisecond)
}

// TestScan_hungTargetDoesNotBlockOthers проверяет, что при двух воркерах
// быстрая цель отвечает, а скан заканчивается вместе с таймаутом зависшей.
func TestScan_hungTargetDoesNotBlockOthers(t *testing.T) {
	const budget = 200 * time.Millisecond
	hung := startServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	fast := startServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	start := time.Now()
	got := Scan(t.Context(), []Target{hung, fast}, false, 2, budget)
	elapsed := time.Since(start)

	require.Len(t, got.Finished, 2)
	assert.False(t, got.interrupted)
	assert.Equal(t, hung, got.Finished[0].Target)
	assert.Equal(t, Down, got.Finished[0].Outcome)
	assert.Equal(t, 0, got.Finished[0].StatusCode)
	assert.Equal(t, "timeout", got.Finished[0].Reason())
	assert.GreaterOrEqual(t, got.Finished[0].Latency, budget)
	assert.Equal(t, fast, got.Finished[1].Target)
	assert.Equal(t, Up, got.Finished[1].Outcome)
	assert.Equal(t, http.StatusOK, got.Finished[1].StatusCode)
	assert.Less(t, elapsed, time.Second)
}

func TestScan_selfSigned(t *testing.T) {
	srv := tlstest.Server(t)

	got := Scan(t.Context(), []Target{Target(srv.URL)}, false, 1, 5*time.Second)

	require.Len(t, got.Finished, 1)
	assert.Equal(t, Target(srv.URL), got.Finished[0].Target)
	assert.Equal(t, Down, got.Finished[0].Outcome)
	assert.Equal(t, 0, got.Finished[0].StatusCode)
	assert.True(t, strings.HasPrefix(got.Finished[0].Reason(), "TLS:"))
}

func TestScan_insecureSelfSigned(t *testing.T) {
	srv := tlstest.Server(t)

	got := Scan(t.Context(), []Target{Target(srv.URL)}, true, 1, 5*time.Second)

	require.Len(t, got.Finished, 1)
	assert.Equal(t, Target(srv.URL), got.Finished[0].Target)
	require.NoError(t, got.Finished[0].Err)
	assert.Equal(t, Up, got.Finished[0].Outcome)
	assert.Equal(t, http.StatusOK, got.Finished[0].StatusCode)
}

func TestScan_cancellation(t *testing.T) {
	var started atomic.Int32
	hang := func(w http.ResponseWriter, r *http.Request) {
		started.Add(1)
		<-r.Context().Done()
	}
	fast := startServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	slow1 := startServer(t, hang)
	slow2 := startServer(t, hang)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	var got ScanResult
	go func() {
		got = Scan(ctx, []Target{fast, slow1, slow2}, false, 1, 5*time.Second)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return started.Load() == 1
	}, time.Second, 5*time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scan did not return after cancel")
	}

	assert.True(t, got.interrupted)
	require.Len(t, got.Finished, 1)
	assert.Equal(t, fast, got.Finished[0].Target)
	require.NoError(t, got.Finished[0].Err)
	assert.Equal(t, Up, got.Finished[0].Outcome)
	assert.Equal(t, http.StatusOK, got.Finished[0].StatusCode)
	assert.Equal(t, Summary{Up: 1}, got.Summary())
	assert.Equal(t, ExitInterrupted, got.Exit())
	assert.Equal(t, int32(1), started.Load())
}

func TestExit(t *testing.T) {
	up := Result{Outcome: Up}
	down := Result{Outcome: Down}
	tests := []struct {
		name string
		scan ScanResult
		want ExitCode
	}{
		{name: "no results", scan: ScanResult{}, want: ExitUsage},
		{name: "one up", scan: ScanResult{Finished: []Result{up}}, want: ExitOK},
		{name: "one down", scan: ScanResult{Finished: []Result{down}}, want: ExitDown},
		{name: "all up", scan: ScanResult{Finished: []Result{up, up}}, want: ExitOK},
		{name: "down after up", scan: ScanResult{Finished: []Result{up, down}}, want: ExitDown},
		{name: "up after down", scan: ScanResult{Finished: []Result{down, up}}, want: ExitDown},
		{name: "all down", scan: ScanResult{Finished: []Result{down, down}}, want: ExitDown},
		{name: "interrupted with no probes", scan: ScanResult{interrupted: true}, want: ExitInterrupted},
		{name: "interrupted with up", scan: ScanResult{interrupted: true, Finished: []Result{up}}, want: ExitInterrupted},
		{name: "interrupted with down", scan: ScanResult{interrupted: true, Finished: []Result{down}}, want: ExitInterrupted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.scan.Exit())
		})
	}
}

func TestScanResult_Summary(t *testing.T) {
	got := ScanResult{
		interrupted: true,
		Finished: []Result{
			{Outcome: Up},
			{Outcome: Down, StatusCode: http.StatusInternalServerError},
		},
	}.Summary()
	assert.Equal(t, Summary{Up: 1, Down: 1}, got)
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
