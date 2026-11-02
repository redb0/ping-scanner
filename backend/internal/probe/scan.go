package probe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type ExitCode int

const (
	ExitOK          ExitCode = 0
	ExitDown        ExitCode = 1
	ExitUsage       ExitCode = 2
	ExitInterrupted ExitCode = 130
)

type Summary struct {
	Up   int
	Down int
}

func (s Summary) Total() int {
	return s.Up + s.Down
}

func (s Summary) String() string {
	return fmt.Sprintf("%d up, %d down, %d total", s.Up, s.Down, s.Total())
}

// ScanResult — итог одного Scan.
// Finished — завершённые Probe в порядке входа. Прерывание снаружи видно как Exit() == ExitInterrupted.
type ScanResult struct {
	Finished    []Result
	interrupted bool
}

func (s ScanResult) Summary() Summary {
	return Summarize(s.Finished)
}

func (s ScanResult) Exit() ExitCode {
	if s.interrupted {
		return ExitInterrupted
	}
	if len(s.Finished) == 0 {
		return ExitUsage
	}
	for _, result := range s.Finished {
		if result.Outcome == Down {
			return ExitDown
		}
	}
	return ExitOK
}

func Summarize(results []Result) Summary {
	var summary Summary
	for _, result := range results {
		if result.Outcome == Up {
			summary.Up++
		} else {
			summary.Down++
		}
	}
	return summary
}

func Scan(
	ctx context.Context,
	targets []Target,
	insecure bool,
	concurrency int,
	timeout time.Duration,
) ScanResult {
	client := httpClient(insecure)
	results := make([]Result, len(targets))
	done := make([]bool, len(targets))
	workers := min(concurrency, len(targets))
	jobs := make(chan int)

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				result := Probe(ctx, targets[i], client, timeout)
				if errors.Is(result.Err, context.Canceled) {
					return
				}
				results[i] = result
				done[i] = true
			}
		})
	}

	enqueue(ctx, jobs, len(targets))
	wg.Wait()

	finished := make([]Result, 0, len(targets))
	for i := range targets {
		if !done[i] {
			continue
		}
		finished = append(finished, results[i])
	}
	return ScanResult{
		Finished:    finished,
		interrupted: ctx.Err() != nil,
	}
}

func enqueue(ctx context.Context, jobs chan<- int, n int) {
	defer close(jobs)
	for i := range n {
		select {
		case <-ctx.Done():
			return
		case jobs <- i:
		}
	}
}
