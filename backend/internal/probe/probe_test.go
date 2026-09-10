package probe

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startServer(t *testing.T, handler http.HandlerFunc) Target {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return Target(srv.URL)
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
			}))

			require.NoError(t, got.Err)
			assert.Equal(t, test.wantOutcome, got.Outcome)
			assert.Equal(t, test.status, got.StatusCode)
		})
	}
}

func TestProbe_refused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	got := Probe(Target("http://" + addr))
	assert.Equal(t, Down, got.Outcome)
	assert.Equal(t, 0, got.StatusCode)
	assert.Error(t, got.Err)
}

func TestProbe_redirect(t *testing.T) {
	got := Probe(startServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/up" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/up", http.StatusFound)
	}))

	require.NoError(t, got.Err)
	assert.Equal(t, Up, got.Outcome)
	assert.Equal(t, http.StatusOK, got.StatusCode)
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

	got := Probe(target)

	require.NoError(t, got.Err)
	assert.Equal(t, Up, got.Outcome)
	assert.Equal(t, http.StatusOK, got.StatusCode)
}
