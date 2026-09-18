package probe

import (
	"context"
	"net/http"
)

const userAgent = "probe/1.0"

type Outcome int

const (
	Down Outcome = iota
	Up
)

// Result — исход одной Probe.
type Result struct {
	Outcome    Outcome
	StatusCode int   // финальный HTTP-код после редиректов; 0, если ответа нет
	Err        error // сырая ошибка; nil, если ответ получен
}

func (r Result) Reason() string {
	return clipReason(reason(r.Err))
}

// Следует редиректам (до 10).
var client = &http.Client{}

func Probe(target Target) Result {
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
