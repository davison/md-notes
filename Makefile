VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build ui test vet check clean

all: build

## build: build the UI and the static mdn binary
build: ui
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o mdn ./cmd/mdn

## ui: install UI dependencies and produce ui/dist
ui:
	pnpm --dir ui install --frozen-lockfile
	pnpm --dir ui build

## test: run Go tests
test:
	go test ./...

## vet: static checks for Go and the UI
vet:
	go vet ./...
	pnpm --dir ui typecheck

## check: everything CI runs
check: vet test build

clean:
	rm -rf mdn ui/dist
