# Keep orchestration and tool versions in internal/cmd/dev.
[windows]
set shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command"]

# List tasks without running checks or changing files.
[default]
list:
    @just --list

# Verify formatting, vet, lint, tests, module tidiness, and API inventory.
check:
    go run ./internal/cmd/dev check

# Run Go tests only; no Docker or static analysis.
test:
    go test ./...

# Run pinned golangci-lint, including test code.
lint:
    go run ./internal/cmd/dev lint

# Explicitly format Go source files (writes files).
fmt:
    go run ./internal/cmd/dev fmt

# Build the host executable.
build:
    go run ./internal/cmd/dev build

# Cross-build all six release targets.
cross:
    go run ./internal/cmd/dev cross

# Run the race detector (requires a supported native C toolchain).
race:
    go run ./internal/cmd/dev race

# Run the pinned vulnerability scanner.
vuln:
    go run ./internal/cmd/dev vuln

# Build snapshot release archives without publishing.
snapshot:
    go run ./internal/cmd/dev snapshot

# Opt in to Git hooks; never installed implicitly.
hooks:
    go run ./internal/cmd/dev hooks
