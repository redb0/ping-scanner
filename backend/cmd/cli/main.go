package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"ping-scanner/internal/probe"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ping-scanner", flag.ContinueOnError)
	fs.SetOutput(stderr)
	failFast := fs.Bool("fail-fast", false, "stop on first invalid target")
	if err := fs.Parse(args); err != nil {
		return int(probe.ExitUsage)
	}

	targets, skipped, err := probe.ParseTargets(fs.Args(), *failFast)
	if skipped > 0 {
		if _, err := fmt.Fprintf(stderr, "skipped %d invalid target(s)\n", skipped); err != nil {
			return int(probe.ExitUsage)
		}
	}
	if err != nil {
		return int(probe.ExitUsage)
	}

	results := probe.Scan(targets)
	for i, result := range results {
		if _, err := fmt.Fprintln(stdout, formatLine(targets[i], result)); err != nil {
			return int(probe.ExitUsage)
		}
	}
	return int(probe.Exit(results))
}

func formatLine(target probe.Target, result probe.Result) string {
	line := fmt.Sprintf(
		"%s %s %d %d",
		target,
		result.Outcome,
		result.StatusCode,
		result.Latency.Milliseconds(),
	)
	if result.Outcome == probe.Down {
		if reason := result.Reason(); reason != "" {
			return line + " " + reason
		}
	}
	return line
}
