package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"ping-scanner/internal/probe"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ping-scanner", flag.ContinueOnError)
	fs.SetOutput(stderr)
	failFast := fs.Bool("fail-fast", false, "stop on first invalid target")
	insecure := fs.Bool("insecure", false, "skip TLS certificate verification")
	filepath := fs.String("f", "", "read targets from file")
	concurrency := fs.Int("concurrency", 10, "number of concurrent requests")
	timeoutFlag := fs.Float64("timeout", probe.DefaultTimeout.Seconds(), "timeout in seconds")
	if err := fs.Parse(args); err != nil {
		return int(probe.ExitUsage)
	}
	if *concurrency < 1 {
		if _, err := fmt.Fprintln(stderr, "concurrency must be greater than 0"); err != nil {
			return int(probe.ExitUsage)
		}
		return int(probe.ExitUsage)
	}
	timeout, err := probe.ValidateTimeout(*timeoutFlag)
	if err != nil {
		if _, err := fmt.Fprintln(stderr, err); err != nil {
			return int(probe.ExitUsage)
		}
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

	scanResult := probe.Scan(ctx, targets, *insecure, *concurrency, timeout)
	for _, result := range scanResult.Finished {
		if _, err := fmt.Fprintln(stdout, formatLine(result)); err != nil {
			return int(probe.ExitUsage)
		}
	}

	summary := scanResult.Summary()
	if len(scanResult.Finished) > 0 {
		if _, err := fmt.Fprintln(stdout, summary.String()); err != nil {
			return int(probe.ExitUsage)
		}
	}

	code := scanResult.Exit()
	if code == probe.ExitInterrupted {
		if _, err := fmt.Fprintln(stdout, "(interrupted)"); err != nil {
			return int(probe.ExitUsage)
		}
	}

	return int(code)
}

func formatLine(result probe.Result) string {
	line := fmt.Sprintf(
		"%s %s %d %d",
		result.Target,
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
