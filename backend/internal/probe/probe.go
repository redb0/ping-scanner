package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"time"
)

const (
	userAgent      = "probe/1.0"
	DefaultTimeout = 5 * time.Second
)

type Outcome int

const (
	Down Outcome = iota
	Up
)

func (o Outcome) String() string {
	if o == Up {
		return "Up"
	}
	return "Down"
}

// Result — исход одной Probe.
type Result struct {
	Target     Target
	Outcome    Outcome
	StatusCode int   // финальный HTTP-код после редиректов; 0, если ответа нет
	Err        error // сырая ошибка; nil, если ответ получен
	Latency    time.Duration
}

func (r Result) Reason() string {
	return clipReason(reason(r.Err))
}

func Probe(ctx context.Context, target Target, client *http.Client, timeout time.Duration) Result {
	start := time.Now()
	result := httpGet(ctx, target, client, timeout)
	result.Target = target
	result.Latency = time.Since(start)
	return result
}

func httpGet(ctx context.Context, target Target, client *http.Client, timeout time.Duration) Result {
	result := Result{Outcome: Down}

	httpCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, string(target), nil)
	if err != nil {
		result.Err = err
		return result
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		result.Err = err
		return result
	}
	cancel()
	_ = resp.Body.Close()

	result.StatusCode = resp.StatusCode
	if resp.StatusCode < 500 {
		result.Outcome = Up
	}
	return result
}

func httpClient(insecure bool) *http.Client {
	return httpClientWithRoots(insecure, nil)
}

func httpClientWithRoots(insecure bool, roots *x509.CertPool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 0
	transport.DialContext = (&net.Dialer{KeepAlive: 0}).DialContext
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: insecure,
		RootCAs:            roots,
	}
	// Следует редиректам (до 10).
	return &http.Client{Timeout: 0, Transport: transport}
}

func ValidateTimeout(timeout float64) (time.Duration, error) {
	if timeout <= 0 {
		return 0, ErrInvalidTimeout
	}
	return time.Duration(timeout * float64(time.Second)), nil
}
