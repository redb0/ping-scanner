package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ping-scanner/internal/probe"
	"ping-scanner/internal/tlstest"
)

func startCLIServer(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func stdoutLine(target, outcome, status, reason string) string {
	line := regexp.QuoteMeta(target) + " " + outcome + " " + status + ` \d+`
	if reason != "" {
		line += " " + regexp.QuoteMeta(reason)
	}
	return line + `\n`
}

func stdoutSummary(up, down int) string {
	return regexp.QuoteMeta(fmt.Sprintf("%d up, %d down, %d total\n", up, down, up+down))
}

func TestRun(t *testing.T) {
	up := startCLIServer(t, http.StatusOK)
	down := startCLIServer(t, http.StatusInternalServerError)

	tests := []struct {
		name          string
		args          []string
		wantCode      int
		stdoutPattern string
		wantStderr    string
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
			name:     "fail-fast",
			args:     []string{"--fail-fast", "ftp://x.io", up},
			wantCode: int(probe.ExitUsage),
		},
		{
			name:          "all up",
			args:          []string{up},
			wantCode:      int(probe.ExitOK),
			stdoutPattern: stdoutLine(up, "Up", "200", "") + stdoutSummary(1, 0),
		},
		{
			name:          "at least one down",
			args:          []string{up, down},
			wantCode:      int(probe.ExitDown),
			stdoutPattern: stdoutLine(up, "Up", "200", "") + stdoutLine(down, "Down", "500", "") + stdoutSummary(1, 1),
		},
		{
			name:          "skip does not force usage exit",
			args:          []string{"ftp://x.io", up},
			wantCode:      int(probe.ExitOK),
			stdoutPattern: stdoutLine(up, "Up", "200", "") + stdoutSummary(1, 0),
			wantStderr:    "skipped 1 invalid target(s)\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(test.args, &stdout, &stderr)
			assert.Equal(t, test.wantCode, got)
			assert.Regexp(t, "^"+test.stdoutPattern+"$", stdout.String())
			assert.Equal(t, test.wantStderr, stderr.String())
		})
	}
}

func TestRun_connectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	raw := "http://" + addr
	var stdout, stderr bytes.Buffer
	got := run([]string{raw}, &stdout, &stderr)

	assert.Equal(t, int(probe.ExitDown), got)
	assert.Regexp(t, "^"+stdoutLine(raw, "Down", "0", "connection refused")+stdoutSummary(0, 1)+"$", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRun_insecure(t *testing.T) {
	srv := tlstest.Server(t)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--insecure", srv.URL}, &stdout, &stderr)

	assert.Equal(t, int(probe.ExitOK), got)
	assert.Regexp(t, "^"+stdoutLine(srv.URL, "Up", "200", "")+stdoutSummary(1, 0)+"$", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRun_unknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run([]string{"--nope"}, &stdout, &stderr)
	assert.Equal(t, int(probe.ExitUsage), got)
	assert.Empty(t, stdout.String())
	assert.NotEmpty(t, stderr.String())
}

func TestFormatLine(t *testing.T) {
	got := formatLine("https://x.io", probe.Result{
		Outcome:    probe.Up,
		StatusCode: http.StatusOK,
		Latency:    42 * time.Millisecond,
	})
	assert.Equal(t, "https://x.io Up 200 42", got)
}
