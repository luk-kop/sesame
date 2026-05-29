APP := sesame
CMD := ./cmd/sesame
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)
GO := GOCACHE=$(CURDIR)/.cache/go-build go

.PHONY: help tidy fmt test build run clean

help:
	@printf '%s\n' 'Available targets:'
	@printf '  %-10s %s\n' 'tidy' 'sync Go module dependencies'
	@printf '  %-10s %s\n' 'fmt' 'format Go sources'
	@printf '  %-10s %s\n' 'test' 'run all tests'
	@printf '  %-10s %s\n' 'build' 'build the sesame binary'
	@printf '  %-10s %s\n' 'run' 'run the CLI help'
	@printf '  %-10s %s\n' 'clean' 'remove build artifacts'

tidy:
	$(GO) mod tidy

fmt:
	gofmt -w cmd internal

test:
	$(GO) test ./...

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) $(CMD)

run:
	$(GO) run $(CMD) --help

clean:
	rm -rf $(BIN_DIR) .cache
