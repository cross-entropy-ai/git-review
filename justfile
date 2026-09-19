set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

version := "1.7.0"
agg := env_var_or_default("AGG", "agg")

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

# Record the guided demo using asciinema, tmux, and Python 3.
record-demo: build
    python3 scripts/record-demo.py

# Render the recording as an animated README preview (requires agg).
demo-gif:
    {{agg}} --quiet --font-size 18 --line-height 1.25 --theme github-dark --fps-cap 12 --idle-time-limit 3 --select 3.. --last-frame-duration 1 docs/demo.cast docs/demo.gif

# Publish the checked recording to asciinema.org.
upload-demo:
    asciinema upload --server-url https://asciinema.org --visibility public --title 'git review — Review your branch. Keep your place.' --description 'A local PR-style review experience for your terminal. Compare inline and split diffs, browse a file tree, resume viewed progress, and toggle between committed and local changes. https://github.com/cross-entropy-ai/git-review' docs/demo.cast

# Install to a chosen directory; default is the user's local bin.
[positional-arguments]
install dest=(env_var("HOME") / ".local/bin"): build
    install -d "$1"
    install -m 755 git-review "$1/git-review"

# Build macOS/Linux amd64/arm64 archives, checksums, an installer, and a Homebrew formula.
[positional-arguments]
dist tag:
    python3 scripts/release.py "$1"

# Verify installer platform selection, checksums, and failed upgrades offline.
test-installer:
    python3 scripts/test-installer.py
