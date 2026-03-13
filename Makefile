.PHONY: build dev clean stop test test-nocache lint

# Build the foobar binary into the bin/ directory
build:
	go build -o bin/foobar ./cmd/foobar

# Run the local development environment using the testdata stubs.
# We first clean up completely to ensure a fresh start, build the latest code, 
# and then run the TUI from within the testdata directory so it picks up the stubs.
dev: clean build
	cd testdata && ../bin/foobar

# Stop the background daemon if it is running
stop:
	@echo "Stopping foobar daemon..."
	@pkill -f "foobar server" || true

# Clean up binaries, sockets, and the test database
clean: stop
	rm -rf bin/
	rm -f /tmp/foobar.sock
	rm -f testdata/foobar.duckdb

# Run the test suite
test:
	go test -v ./...

# Run the test suite without using the Go test cache
test-nocache:
	go test -count=1 -v ./...

# Run linters on the codebase
lint:
	go vet ./...
	go fmt ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping advanced linting."; \
	fi
