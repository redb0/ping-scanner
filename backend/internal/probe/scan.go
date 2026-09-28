package probe

import (
	"fmt"
	"sync"
)

type ExitCode int

const (
	ExitOK    ExitCode = 0
	ExitDown  ExitCode = 1
	ExitUsage ExitCode = 2
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

func Scan(targets []Target, insecure bool, concurrency int) []Result {
	client := httpClient(insecure)
	results := make([]Result, len(targets))
	workers := min(concurrency, len(targets))
	jobs := make(chan int)

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				results[i] = Probe(targets[i], client)
			}
		})
	}

	for i := range targets {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
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
