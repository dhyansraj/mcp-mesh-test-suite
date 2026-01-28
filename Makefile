.PHONY: build build-cli build-runner run clean deps test build-dashboard build-with-dashboard

# Version can be overridden: make build VERSION=1.2.3
VERSION ?= dev

# Build both binaries (CLI and runner) - native for current platform
build: build-cli build-runner

# Build just the CLI binary
build-cli:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/tsuite ./cmd/tsuite

# Build dashboard from Next.js source
build-dashboard:
	cd dashboard && npm run build
	rm -rf cmd/tsuite/dashboard
	cp -r dashboard/out cmd/tsuite/dashboard

# Build CLI with embedded dashboard
build-with-dashboard: build-dashboard build-cli

# Build just the runner binary (native - for npm package on Linux)
build-runner:
	go build -o bin/tsuite-runner ./cmd/runner

# Run the API server
run: build
	./bin/tsuite api --port 9999

# Install dependencies
deps:
	go mod tidy
	go mod download

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test -v ./...

# Build runner for Linux (for Docker mode development on Mac)
# Only needed when developing on Mac and running Docker tests
# Uses host architecture (arm64 on M1/M2, amd64 on Intel)
build-runner-linux:
	GOOS=linux go build -o bin/tsuite-runner-linux ./cmd/runner
