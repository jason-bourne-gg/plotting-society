#!/usr/bin/env bash
# Runs the Go toolchain in a container, so the repo needs no local Go install.
#
# When the docker-compose stack is up, the container joins its network and the
# database is reachable as `postgres`, which is how the integration tests get a
# real Postgres without anything installed on the host.
#
#   ./go.sh build ./...
#   ./go.sh test ./... -cover
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
net_args=()
if docker network inspect plotting-society_default >/dev/null 2>&1; then
  net_args=(--network plotting-society_default)
fi

exec docker run --rm \
  -v "$root":/src -w /src \
  -v go-mod-cache:/go/pkg/mod -v go-build-cache:/root/.cache/go-build \
  -e TEST_DATABASE_URL="${TEST_DATABASE_URL:-postgres://plot:plot@postgres:5432/plotting?sslmode=disable}" \
  "${net_args[@]}" \
  golang:1.23-alpine go "$@"
