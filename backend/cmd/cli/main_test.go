package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
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

func writeTargets(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "targets.txt")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
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
	fromFile := writeTargets(t, "# comment\n\n"+up+"\n")
	invalidInFile := writeTargets(t, "ftp://x.io\n")
	mergeFile := writeTargets(t, up+"\n")
	failFastFile := writeTargets(t, "ftp://x.io\n"+up+"\n")
	missing := filepath.Join(t.TempDir(), "missing.txt")

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
		{
			name:          "file targets",
			args:          []string{"-f", fromFile},
			wantCode:      int(probe.ExitOK),
			stdoutPattern: stdoutLine(up, "Up", "200", "") + stdoutSummary(1, 0),
		},
		{
			name:          "invalid file line is skipped",
			args:          []string{"-f", invalidInFile, up},
			wantCode:      int(probe.ExitOK),
			stdoutPattern: stdoutLine(up, "Up", "200", "") + stdoutSummary(1, 0),
			wantStderr:    "skipped 1 invalid target(s)\n",
		},
		{
			name:          "file and args merge",
			args:          []string{"-f", mergeFile, down},
			wantCode:      int(probe.ExitDown),
			stdoutPattern: stdoutLine(up, "Up", "200", "") + stdoutLine(down, "Down", "500", "") + stdoutSummary(1, 1),
		},
		{
			name:          "dedup file and args",
			args:          []string{"-f", mergeFile, down, up},
			wantCode:      int(probe.ExitDown),
			stdoutPattern: stdoutLine(up, "Up", "200", "") + stdoutLine(down, "Down", "500", "") + stdoutSummary(1, 1),
		},
		{
			name:     "fail-fast on file line",
			args:     []string{"--fail-fast", "-f", failFastFile},
			wantCode: int(probe.ExitUsage),
		},
		{
			name:       "missing file",
			args:       []string{"-f", missing, up},
			wantCode:   int(probe.ExitUsage),
			wantStderr: "open " + missing + ": no such file or directory\n",
		},
		{
			name:          "concurrency keeps order and exit",
			args:          []string{"--concurrency", "4", up, down},
			wantCode:      int(probe.ExitDown),
			stdoutPattern: stdoutLine(up, "Up", "200", "") + stdoutLine(down, "Down", "500", "") + stdoutSummary(1, 1),
		},
		{
			name:       "concurrency must be positive",
			args:       []string{"--concurrency", "0", up},
			wantCode:   int(probe.ExitUsage),
			wantStderr: "concurrency must be greater than 0\n",
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

func TestRun_concurrencyOrder(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(slow.Close)
	fast := startCLIServer(t, http.StatusInternalServerError)

	var stdout, stderr bytes.Buffer
	got := run([]string{"--concurrency", "2", slow.URL, fast}, &stdout, &stderr)

	assert.Equal(t, int(probe.ExitDown), got)
	assert.Regexp(t, "^"+stdoutLine(slow.URL, "Up", "200", "")+stdoutLine(fast, "Down", "500", "")+stdoutSummary(1, 1)+"$", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRun_defaultConcurrency(t *testing.T) {
	const n = 10
	var current, maxSeen atomic.Int32
	release := make(chan struct{})
	var once sync.Once
	releaseAll := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseAll)

	urls := make([]string, n)
	for i := range n {
		status := http.StatusOK + i
		urls[i] = startGateServer(t, status, &current, &maxSeen, release)
	}

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(urls, &stdout, &stderr)
	}()

	require.Eventually(t, func() bool {
		return current.Load() == n
	}, 2*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(n), maxSeen.Load())
	releaseAll()

	var got int
	select {
	case got = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish")
	}
	assert.Equal(t, int(probe.ExitOK), got)
	assert.Empty(t, stderr.String())

	var pattern strings.Builder
	for i, raw := range urls {
		pattern.WriteString(stdoutLine(raw, "Up", fmt.Sprintf("%d", http.StatusOK+i), ""))
	}
	pattern.WriteString(stdoutSummary(n, 0))
	assert.Regexp(t, "^"+pattern.String()+"$", stdout.String())
}

func startGateServer(t *testing.T, status int, current, maxSeen *atomic.Int32, release <-chan struct{}) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		seen := current.Add(1)
		for {
			old := maxSeen.Load()
			if seen <= old || maxSeen.CompareAndSwap(old, seen) {
				break
			}
		}
		<-release
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
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
