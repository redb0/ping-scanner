package probe

import (
	"errors"
	"net/url"
	"strings"
)

type Target string

func ParseTargets(raws []string, failFast bool) ([]Target, int, error) {
	seen := make(map[Target]struct{}, len(raws))
	targets := make([]Target, 0, len(raws))
	skipped := 0
	for _, raw := range raws {
		target, err := parseTarget(raw)
		if err != nil {
			if errors.Is(err, ErrEmptyTarget) {
				continue
			}
			if failFast {
				return nil, 0, err
			}
			skipped++
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	return targets, skipped, nil
}

func parseTarget(raw string) (Target, error) {
	trimRaw := strings.TrimSpace(raw)
	if trimRaw == "" {
		return "", ErrEmptyTarget
	}
	if !strings.Contains(trimRaw, "://") {
		if strings.Contains(trimRaw, ":") {
			return "", ErrInvalidTarget
		}
		trimRaw = "https://" + trimRaw
	}
	parsedURL, err := url.ParseRequestURI(trimRaw)
	if err != nil {
		return "", ErrInvalidTarget
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", ErrInvalidTarget
	}
	if parsedURL.Host == "" {
		return "", ErrInvalidTarget
	}
	return Target(parsedURL.String()), nil
}
