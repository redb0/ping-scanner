lint:
    #!/usr/bin/env bash
    set -euo pipefail
    cd backend
    unformatted="$(gofmt -l -s .)"
    if [ -n "$unformatted" ]; then
      echo "These files are not gofmt-formatted:"
      printf '%s\n' "$unformatted"
      exit 1
    fi
    golangci-lint run
