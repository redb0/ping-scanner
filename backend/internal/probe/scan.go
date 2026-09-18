package probe

type ExitCode int

const (
	ExitOK    ExitCode = 0
	ExitDown  ExitCode = 1
	ExitUsage ExitCode = 2
)

func Scan(targets []Target) []Result {
	results := make([]Result, len(targets))
	for i, target := range targets {
		results[i] = Probe(target)
	}
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
