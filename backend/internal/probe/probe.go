package probe

import (
	"context"
	"net/http"
	"time"
)

const userAgent = "probe/1.0"

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
	Outcome    Outcome
	StatusCode int   // финальный HTTP-код после редиректов; 0, если ответа нет
	Err        error // сырая ошибка; nil, если ответ получен
	Latency    time.Duration
}

func (r Result) Reason() string {
	return clipReason(reason(r.Err))
}

const clientTimeout = 5 * time.Second

// Следует редиректам (до 10).
var client = &http.Client{Timeout: clientTimeout}

func Probe(target Target) Result {
	start := time.Now()
	result := httpGet(target)
	result.Latency = time.Since(start)
	return result
}

func httpGet(target Target) Result {
	result := Result{Outcome: Down}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, string(target), nil)
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
