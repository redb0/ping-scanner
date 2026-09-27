package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"ping-scanner/internal/probe"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ping-scanner", flag.ContinueOnError)
	fs.SetOutput(stderr)
	failFast := fs.Bool("fail-fast", false, "stop on first invalid target")
	insecure := fs.Bool("insecure", false, "skip TLS certificate verification")
	filepath := fs.String("f", "", "read targets from file")
	if err := fs.Parse(args); err != nil {
		return int(probe.ExitUsage)
	}

	raws, err := loadTargets(*filepath, fs.Args())
	if err != nil {
		if _, err := fmt.Fprintln(stderr, err); err != nil {
			return int(probe.ExitUsage)
		}
		return int(probe.ExitUsage)
	}

	targets, skipped, err := probe.ParseTargets(raws, *failFast)
	if skipped > 0 {
		if _, err := fmt.Fprintf(stderr, "skipped %d invalid target(s)\n", skipped); err != nil {
			return int(probe.ExitUsage)
		}
	}
	if err != nil {
		return int(probe.ExitUsage)
	}

	results := probe.Scan(targets, *insecure)
	for i, result := range results {
		if _, err := fmt.Fprintln(stdout, formatLine(targets[i], result)); err != nil {
			return int(probe.ExitUsage)
		}
	}

	summary := probe.Summarize(results)
	if len(results) > 0 {
		if _, err := fmt.Fprintln(stdout, summary.String()); err != nil {
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

func loadTargets(filepath string, raws []string) ([]string, error) {
	if filepath == "" {
		return raws, nil
	}
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	rawTargets := make([]string, 0, len(raws)+len(lines))
	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "#") {
			continue
		}
		rawTargets = append(rawTargets, raw)
	}
	rawTargets = append(rawTargets, raws...)
	return rawTargets, nil
}
