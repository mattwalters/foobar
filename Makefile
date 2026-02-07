# Binary name
BINARY_NAME=foobar
CMD_PATH=./cmd/foobar

# Go related variables.
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin
GOFILES=$(wildcard *.go)

# Make is verbose in Linux. Make it silent.
MAKEFLAGS += --silent

.PHONY: all build install run test clean help

## all: Build and run tests
all: test build

## build: Build the binary to ./bin
build:
	@echo "  >  Building binary..."
	go build -o ./bin/$(BINARY_NAME) $(CMD_PATH)
	@echo "  >  Built content ./bin/$(BINARY_NAME)"

## install: Install the binary to $GOPATH/bin
install:
	@echo "  >  Installing binary..."
	go install $(CMD_PATH)
	@echo "  >  Installed to $$(go env GOPATH)/bin/$(BINARY_NAME)"

## run: Run the application
run:
	go run $(CMD_PATH)

## test: Run unit and integration tests
test:
	@echo "  >  Running tests..."
	go test -v ./internal/...

## clean: Clean build artifacts
clean:
	@echo "  >  Cleaning build cache..."
	go clean
	rm -rf ./bin

## help: Show help
help: Makefile
	@echo
	@echo " Choose a command run in "$(PROJECTNAME)":"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo
