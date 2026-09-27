package probe

import (
	"net/http"
	"strings"
	"testing"

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

			got := Scan(targets, false)

			outcomes := make([]Outcome, len(got))
			for i, result := range got {
				outcomes[i] = result.Outcome
			}
			assert.Equal(t, test.want, outcomes)
		})
	}
}

func TestScan_selfSigned(t *testing.T) {
	srv := tlstest.Server(t)

	got := Scan([]Target{Target(srv.URL)}, false)

	require.Len(t, got, 1)
	assert.Equal(t, Down, got[0].Outcome)
	assert.Equal(t, 0, got[0].StatusCode)
	assert.True(t, strings.HasPrefix(got[0].Reason(), "TLS:"))
}

func TestScan_insecureSelfSigned(t *testing.T) {
	srv := tlstest.Server(t)

	got := Scan([]Target{Target(srv.URL)}, true)

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
