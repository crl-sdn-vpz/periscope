set shell := ["bash", "-euo", "pipefail", "-c"]

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X main.version=" + version
targets := "linux/amd64 linux/arm64 windows/amd64 darwin/arm64 darwin/amd64"

# List recipes
default:
    @just --list --unsorted

# Build periscope for this machine into bin/
build:
    go build -ldflags "{{ldflags}}" -o bin/periscope ./cmd/periscope

# Unit tests
test:
    go test ./...

# gofmt + go vet on every release OS + shellcheck
lint:
    test -z "$(gofmt -l .)" || { gofmt -l .; echo "run: gofmt -w ."; exit 1; }
    for os in linux windows darwin; do GOOS=$os go vet ./...; done
    shellcheck -S warning linux/d1 linux/install.sh install/install.sh tests/e2e-xvfb.sh tests/e2e-go-xvfb.sh

# End-to-end tests on a virtual X display (deps: see README > Development)
e2e:
    tests/e2e-xvfb.sh
    tests/e2e-go-xvfb.sh

# Everything CI and reviewers expect to pass
check: lint test e2e

# Cross-compile release binaries and SHA256SUMS into dist/
dist:
    rm -rf dist && mkdir dist
    for t in {{targets}}; do \
        os=${t%/*} arch=${t#*/} ext=""; [[ $os == windows ]] && ext=.exe; \
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "{{ldflags}}" \
            -o "dist/periscope_${os}_${arch}${ext}" ./cmd/periscope; \
    done
    cd dist && sha256sum periscope_* > SHA256SUMS

# Publish dist/ as a GitHub release for an existing tag (external write: needs the owner's OK)
release tag: dist
    git rev-parse -q --verify "refs/tags/{{tag}}" >/dev/null || { echo "tag {{tag}} doesn't exist"; exit 1; }
    gh release create "{{tag}}" dist/* --verify-tag --title "{{tag}}" --generate-notes
