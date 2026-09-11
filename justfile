set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

version := "0.1.0"

# Show available development commands.
default:
    @just --list

# Download and reconcile pinned Go dependencies.
deps:
    go mod tidy

# Compile one self-contained binary (Git must be installed at runtime).
build:
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X main.version={{version}}' -o git-review ./cmd

# Run the TUI. Example: just run --base main
[positional-arguments]
run *args:
    go run ./cmd "$@"

# Run all unit and real-Git integration tests.
test:
    go test ./...

# Check for data races.
test-race:
    go test -race ./...

# Format Go source files.
fmt:
    go fmt ./...

# Check formatting and run Go's static analyzer.
lint:
    @test -z "$(gofmt -l cmd internal)" || { gofmt -l cmd internal; exit 1; }
    go vet ./...

# Full local verification.
check: lint test build

# Create an ignored, isolated Git repository with representative changes.
fixture:
    bash scripts/create-fixture.sh

# Open the sample review (creates the fixture if it does not exist).
demo: build fixture
    ./git-review -C .review-fixture

# Install to a chosen directory; default is the user's local bin.
[positional-arguments]
install dest=(env_var("HOME") / ".local/bin"): build
    install -d "$1"
    install -m 755 git-review "$1/git-review"
