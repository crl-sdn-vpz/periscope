set shell := ["bash", "-euo", "pipefail", "-c"]

# List recipes
default:
    @just --list --unsorted

# Lint the bash scripts
lint:
    shellcheck -S warning linux/d1 linux/install.sh tests/e2e-xvfb.sh

# End-to-end viewer test on a virtual X display (deps: see README > Development)
e2e:
    tests/e2e-xvfb.sh

# Everything CI and reviewers expect to pass
check: lint e2e
