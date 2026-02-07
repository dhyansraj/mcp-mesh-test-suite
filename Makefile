.PHONY: build build-cli build-runner run clean deps test build-dashboard build-with-dashboard build-runner-linux-amd64 build-runner-linux-arm64 build-runner-linux-all build-runner-darwin-amd64 build-runner-darwin-arm64 build-runner-all build-with-runners prepare-runners build-all

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

# Build runner for specific Linux architectures
build-runner-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -o bin/tsuite-runner-linux-amd64 ./cmd/runner

build-runner-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -o bin/tsuite-runner-linux-arm64 ./cmd/runner

# Build runner for all Linux architectures
build-runner-linux-all: build-runner-linux-amd64 build-runner-linux-arm64

# Build runner for specific Darwin architectures
build-runner-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -o bin/tsuite-runner-darwin-amd64 ./cmd/runner

build-runner-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -o bin/tsuite-runner-darwin-arm64 ./cmd/runner

# Build runner for all platforms and architectures
build-runner-all: build-runner-linux-amd64 build-runner-linux-arm64 build-runner-darwin-amd64 build-runner-darwin-arm64

# Prepare runner embed directory (copy built runners + select-runner script)
prepare-runners: build-runner-all
	mkdir -p cmd/tsuite/runners
	cp bin/tsuite-runner-linux-amd64 cmd/tsuite/runners/
	cp bin/tsuite-runner-linux-arm64 cmd/tsuite/runners/
	cp bin/tsuite-runner-darwin-amd64 cmd/tsuite/runners/
	cp bin/tsuite-runner-darwin-arm64 cmd/tsuite/runners/
	cp scripts/select-runner.sh cmd/tsuite/runners/select-runner

# Build CLI with embedded runner binaries
build-with-runners: prepare-runners build-cli

# Build everything (dashboard + runners)
build-all: build-dashboard prepare-runners build-cli build-runner
