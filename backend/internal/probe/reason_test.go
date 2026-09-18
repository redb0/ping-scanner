package probe

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestReason(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "no.example"}
	dnsTimeout := &net.DNSError{Err: "i/o timeout", Name: "slow.example", IsTimeout: true}
	refusedOp := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	tlsUnknown := x509.UnknownAuthorityError{}
	tlsExpired := x509.CertificateInvalidError{Reason: x509.Expired}
	tlsHostname := x509.HostnameError{Host: "x.io", Certificate: &x509.Certificate{}}

	_, invalidURLErr := http.NewRequest(http.MethodGet, "http://[::1]:namedport", nil)
	require.Error(t, invalidURLErr)

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil",
			err:  nil,
			want: "",
		},
		{
			name: "deadline",
			err:  context.DeadlineExceeded,
			want: "timeout",
		},
		{
			name: "os deadline",
			err:  os.ErrDeadlineExceeded,
			want: "timeout",
		},
		{
			name: "url deadline",
			err:  &url.Error{Op: "Get", URL: "https://x.io", Err: context.DeadlineExceeded},
			want: "timeout",
		},
		{
			name: "net i/o timeout",
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: timeoutError{}},
			want: "timeout",
		},
		{
			name: "dns timeout",
			err:  dnsTimeout,
			want: "timeout",
		},
		{
			name: "dns no such host",
			err:  dnsErr,
			want: "DNS: no such host",
		},
		{
			name: "url dns",
			err:  &url.Error{Op: "Get", URL: "https://x.io", Err: dnsErr},
			want: "DNS: no such host",
		},
		{
			name: "wrapped dns",
			err:  fmt.Errorf("get: %w", dnsErr),
			want: "DNS: no such host",
		},
		{
			name: "refused",
			err:  syscall.ECONNREFUSED,
			want: "connection refused",
		},
		{
			name: "op refused",
			err:  refusedOp,
			want: "connection refused",
		},
		{
			name: "url refused",
			err:  &url.Error{Op: "Get", URL: "https://x.io", Err: refusedOp},
			want: "connection refused",
		},
		{
			name: "tls unknown authority",
			err:  tlsUnknown,
			want: "TLS: certificate signed by unknown authority",
		},
		{
			name: "tls expired",
			err:  tlsExpired,
			want: "TLS: certificate has expired",
		},
		{
			name: "url tls",
			err:  &url.Error{Op: "Get", URL: "https://x.io", Err: tlsUnknown},
			want: "TLS: certificate signed by unknown authority",
		},
		{
			name: "tls hostname",
			err:  tlsHostname,
			want: "TLS: certificate is not valid for any names, but wanted to match x.io",
		},
		{
			name: "canceled",
			err:  context.Canceled,
			want: "context canceled",
		},
		{
			name: "plain",
			err:  errors.New("weird"),
			want: "weird",
		},
		{
			name: "url other",
			err:  &url.Error{Op: "Get", URL: "https://x.io", Err: errors.New("weird")},
			want: "weird",
		},
		{
			name: "invalid url",
			err:  invalidURLErr,
			want: invalidURLErr.Error(),
		},
		{
			name: "not truncated",
			err:  errors.New(strings.Repeat("a", 80)),
			want: strings.Repeat("a", 80),
		},
		{
			name: "truncated",
			err:  errors.New(strings.Repeat("a", 81)),
			want: strings.Repeat("a", 77) + "...",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, Result{Err: test.err}.Reason())
		})
	}
}
