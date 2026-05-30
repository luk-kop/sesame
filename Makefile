APP := sesame
CMD := ./cmd/sesame
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)
GO := GOCACHE=$(CURDIR)/.cache/go-build go

.PHONY: help tidy update-patch update-minor fmt test build run clean

help:
	@printf '%s\n' 'Available targets:'
	@printf '  %-13s %s\n' 'tidy' 'sync Go module dependencies'
	@printf '  %-13s %s\n' 'update-patch' 'update dependencies (patch only, safe)'
	@printf '  %-13s %s\n' 'update-minor' 'update dependencies (minor + patch)'
	@printf '  %-13s %s\n' 'fmt' 'format Go sources'
	@printf '  %-13s %s\n' 'test' 'run all tests'
	@printf '  %-13s %s\n' 'build' 'build the sesame binary'
	@printf '  %-13s %s\n' 'run' 'run the CLI help'
	@printf '  %-13s %s\n' 'clean' 'remove build artifacts'

tidy:
	$(GO) mod tidy

update-patch:
	$(GO) get -u=patch ./...
	$(GO) mod tidy

update-minor:
	$(GO) get -u ./...
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
