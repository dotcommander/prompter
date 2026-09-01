# Prompter task runner
export GOWORK := "off"

# List available commands
default:
    @just --list

# Build local prompter binary
build:
    go build -o prompter .

# Run all unit tests
test:
    go test -count=1 ./...

# Run documentation consistency tests
test-doctests:
    go test -count=1 ./doctests/...

# Run all unit tests with race detection and doctests
test-all:
    go test -count=1 -race ./...
    go test -count=1 ./doctests/...

# Static analysis
vet:
    go vet ./...

# Format all Go source files
fmt:
    gofmt -w .

# Check formatting without modifying files
fmt-check:
    @test -z "$(gofmt -l .)" || (echo "Unformatted files found:\n$$(gofmt -l .)" && exit 1)

# Run full QA pipeline (format check, vet, unit tests, doctests, build)
qa: fmt-check vet test test-doctests build
    @echo "✓ All QA checks passed!"

# Clean build artifacts
clean:
    rm -f prompter

# Dry-run or publish a resumable source and Homebrew release
release version *ARGS:
    go run ./tools/release {{quote(version)}} {{ARGS}}

# Launch interactive configuration wizard
config:
    go run . configure

# Run prompter with arbitrary arguments (e.g. just run refine --dry-run "test")
run *ARGS:
    go run . {{ARGS}}
