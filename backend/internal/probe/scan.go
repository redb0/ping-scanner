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
) ([]Result, bool) {
	client := httpClient(insecure)
	results := make([]Result, len(targets))
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
			}
		})
	}

	enqueue(ctx, jobs, len(targets))
	wg.Wait()

	return results, ctx.Err() != nil
}

func Exit(results []Result) ExitCode {
	if len(results) == 0 {
		return ExitUsage
	}
	for _, result := range results {
		if result.Outcome == Down {
			return ExitDown
		}
	}
	return ExitOK
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
