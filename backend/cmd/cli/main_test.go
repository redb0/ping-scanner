package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"ping-scanner/internal/probe"
)

func startCLIServer(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRun(t *testing.T) {
	up := startCLIServer(t, http.StatusOK)
	down := startCLIServer(t, http.StatusInternalServerError)

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:     "no targets",
			args:     []string{},
			wantCode: int(probe.ExitUsage),
		},
		{
			name:       "skip all invalid",
			args:       []string{"ftp://x.io"},
			wantCode:   int(probe.ExitUsage),
			wantStderr: "skipped 1 invalid target(s)\n",
		},
		{
			name:       "fail-fast",
			args:       []string{"--fail-fast", "ftp://x.io", up},
			wantCode:   int(probe.ExitUsage),
			wantStdout: "",
		},
		{
			name:       "all up",
			args:       []string{up},
			wantCode:   int(probe.ExitOK),
			wantStdout: up + " Up 200\n",
		},
		{
			name:       "at least one down",
			args:       []string{up, down},
			wantCode:   int(probe.ExitDown),
			wantStdout: fmt.Sprintf("%s Up 200\n%s Down 500\n", up, down),
		},
		{
			name:       "skip does not force usage exit",
			args:       []string{"ftp://x.io", up},
			wantCode:   int(probe.ExitOK),
			wantStdout: up + " Up 200\n",
			wantStderr: "skipped 1 invalid target(s)\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(test.args, &stdout, &stderr)
			assert.Equal(t, test.wantCode, got)
			assert.Equal(t, test.wantStdout, stdout.String())
			assert.Equal(t, test.wantStderr, stderr.String())
		})
	}
}
