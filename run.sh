#!/bin/bash
set -euo pipefail

# Project runner for clother-ng. Usage: ./run.sh <command> [args...]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

app_version() {
    git describe --always --match "v*" 2>/dev/null | sed -E 's/^(v[0-9]+\.[0-9]+)-([0-9]+)-g[0-9a-f]+$/\1.\2/; s/^(v[0-9]+\.[0-9]+)$/\1.0/'
}

CMD="${1:-help}"
shift || true

case "${CMD}" in
    build|b)
        mkdir -p dist
        go build -ldflags "-X github.com/jolehuit/clother/internal/version.Value=$(app_version | sed 's/^v//')" \
            -o dist/clother ./cmd/clother "$@"
        echo "built dist/clother ($(app_version))"
        ;;
    run|r)
        go run ./cmd/clother "$@"
        ;;
    test|t)
        go test ./... "$@"
        ;;
    race)
        go test -race ./... "$@"
        ;;
    lint|l)
        go vet ./...
        bash -n clother.sh scripts/install.sh scripts/package-release.sh run.sh
        ;;
    check|c)
        go vet ./...
        go test -race ./...
        bash -n clother.sh scripts/install.sh scripts/package-release.sh run.sh
        ;;
    cover)
        go test -coverprofile=dist/cover.out ./... "$@"
        go tool cover -func=dist/cover.out | tail -1
        ;;
    version|v)
        app_version
        ;;
    sync-upstream)
        git fetch upstream --no-tags
        git merge --no-edit upstream/main
        ;;
    clean)
        rm -rf dist
        ;;
    help|*)
        echo "Usage: ./run.sh <command>"
        echo "  build          build dist/clother with the git-describe version"
        echo "  run [args]     go run the clother CLI"
        echo "  test [args]    go test ./..."
        echo "  race           go test -race ./..."
        echo "  lint           go vet + bash -n"
        echo "  check          vet + race tests + bash -n (what CI runs)"
        echo "  cover          test with coverage summary"
        echo "  version        print vX.Y.N"
        echo "  sync-upstream  fetch and merge upstream/main"
        echo "  clean          remove dist/"
        ;;
esac
