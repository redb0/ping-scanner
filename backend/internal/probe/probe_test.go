package probe

import (
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ping-scanner/internal/tlstest"
)

func startServer(t *testing.T, handler http.HandlerFunc) Target {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return Target(srv.URL)
}

func trustedClient(cert *x509.Certificate) *http.Client {
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return httpClientWithRoots(false, pool)
}

func TestProbe(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		wantOutcome Outcome
	}{
		{
			name:        "status 200 - Up",
			status:      http.StatusOK,
			wantOutcome: Up,
		},
		{
			name:        "status 404 - Up",
			status:      http.StatusNotFound,
			wantOutcome: Up,
		},
		{
			name:        "status 500 - Down",
			status:      http.StatusInternalServerError,
			wantOutcome: Down,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Probe(startServer(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, userAgent, r.UserAgent())
				w.WriteHeader(test.status)
			}), httpClient(false))

			require.NoError(t, got.Err)
			assert.Equal(t, test.wantOutcome, got.Outcome)
			assert.Equal(t, test.status, got.StatusCode)
		})
	}
}

func TestProbe_invalidURL(t *testing.T) {
	got := Probe(Target("http://[::1]:namedport"), httpClient(false))
	assert.Equal(t, Down, got.Outcome)
	assert.Equal(t, 0, got.StatusCode)
	assert.Error(t, got.Err)
}

func TestProbe_refused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	got := Probe(Target("http://"+addr), httpClient(false))
	assert.Equal(t, Down, got.Outcome)
	assert.Equal(t, 0, got.StatusCode)
	assert.Error(t, got.Err)
	assert.Greater(t, got.Latency, time.Duration(0))
	assert.Less(t, got.Latency, clientTimeout)
}

func TestProbe_redirect(t *testing.T) {
	got := Probe(startServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/up" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/up", http.StatusFound)
	}), httpClient(false))

	require.NoError(t, got.Err)
	assert.Equal(t, Up, got.Outcome)
	assert.Equal(t, http.StatusOK, got.StatusCode)
}

func TestProbe_timeout(t *testing.T) {
	target := startServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	got := Probe(target, httpClient(false))

	assert.Equal(t, Down, got.Outcome)
	assert.Equal(t, 0, got.StatusCode)
	assert.Equal(t, "timeout", got.Reason())
	assert.GreaterOrEqual(t, got.Latency, clientTimeout)
}

func TestProbe_bodyNotDownloaded(t *testing.T) {
	hold := make(chan struct{})
	target := startServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		<-hold
	})
	t.Cleanup(func() { close(hold) })

	got := Probe(target, httpClient(false))

	require.NoError(t, got.Err)
	assert.Equal(t, Up, got.Outcome)
	assert.Equal(t, http.StatusOK, got.StatusCode)
}

func TestProbe_latencySlowGreaterThanFast(t *testing.T) {
	fast := startServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	slow := startServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	client := httpClient(false)
	assert.Greater(t, Probe(slow, client).Latency, Probe(fast, client).Latency)
}

func TestProbe_trustedCertificate(t *testing.T) {
	srv := tlstest.Server(t)

	got := Probe(Target(srv.URL), trustedClient(srv.Certificate()))

	require.NoError(t, got.Err)
	assert.Equal(t, Up, got.Outcome)
	assert.Equal(t, http.StatusOK, got.StatusCode)
}

func TestOutcome_String(t *testing.T) {
	tests := []struct {
		name    string
		outcome Outcome
		want    string
	}{
		{name: "Up", outcome: Up, want: "Up"},
		{name: "Down", outcome: Down, want: "Down"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.outcome.String())
		})
	}
}
