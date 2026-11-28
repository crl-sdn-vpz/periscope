set shell := ["bash", "-euo", "pipefail", "-c"]

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X main.version=" + version
# Release targets: the platforms a role runs on today (macOS is vetted in lint, not shipped yet).
targets := "linux/amd64 linux/arm64 windows/amd64"

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
    shellcheck -S warning linux/d1 linux/install.sh install/install.sh scripts/test-pair.sh scripts/make-icons.sh scripts/make-screenshots.sh tests/installers.sh tests/e2e-xvfb.sh tests/e2e-go-xvfb.sh

# End-to-end tests on a virtual X display (deps: see README > Development)
e2e:
    tests/e2e-xvfb.sh
    tests/e2e-go-xvfb.sh
    tests/installers.sh

# Known vulnerabilities in the code and the Go toolchain (govulncheck)
vuln:
    go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...

# Everything CI and reviewers expect to pass
check: lint test e2e

# Render assets/icon.svg to assets/icon-256.png and assets/periscope.ico
icons:
    scripts/make-icons.sh

# Regenerate the README images from a real run on virtual displays (see the script for its deps)
screenshots:
    scripts/make-screenshots.sh

# Cross-compile release binaries and SHA256SUMS into dist/
dist:
    rm -rf dist && mkdir dist
    for t in {{targets}}; do \
        os=${t%/*} arch=${t#*/} ext=""; [[ $os == windows ]] && ext=.exe; \
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "{{ldflags}}" \
            -o "dist/periscope_${os}_${arch}${ext}" ./cmd/periscope; \
    done
    cp assets/icon-256.png dist/periscope.png
    cp assets/periscope.ico dist/periscope.ico
    cd dist && sha256sum periscope* > SHA256SUMS

# Hardware test of this checkout: serve dist/ on the LAN, start the viewer, print the Windows command
test-pair http_port="8000":
    scripts/test-pair.sh {{http_port}}

# Publish dist/ as a GitHub release for an existing tag (external write: needs the owner's OK)
release tag: dist
    git rev-parse -q --verify "refs/tags/{{tag}}" >/dev/null || { echo "tag {{tag}} doesn't exist"; exit 1; }
    awk -v v="[{{tag}}]" '$1 == "##" && $2 == v {on = 1; next} $1 == "##" && on {exit} on' CHANGELOG.md > dist/NOTES.md
    test -s dist/NOTES.md || { echo "CHANGELOG.md has no section for {{tag}}"; exit 1; }
    gh release create "{{tag}}" dist/periscope* dist/SHA256SUMS --verify-tag --title "periscope {{tag}}" --notes-file dist/NOTES.md
