# Ping Scanner

A tool (CLI in phase 1, web service in phase 2) that checks the availability of a set of web targets by performing an HTTP probe against each and reporting which are up or down.

## Language

**Target**:
A probe destination, normalized to a full URL (bare domains get `https` by default). The unit of input — what you hand the tool.
_Avoid_: URL, site, host

**Probe**:
Performing a single HTTP request against one Target to observe its response.
_Avoid_: ping, check, request

**Scan**:
One full pass of the tool over its entire set of Targets in a single invocation.
_Avoid_: run, job, batch

**Up**:
A Probe's outcome when it received an HTTP response with a status below 500 (after following redirects) within the time budget.
_Avoid_: available, alive, ok

**Down**:
A Probe's outcome when it received a 5xx response, or no response at all — timeout, DNS failure, connection refused, or TLS error.
_Avoid_: dead, unreachable, failed