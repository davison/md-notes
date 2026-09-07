VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build ui ui-deps test vet check install clean
PREFIX ?= $(HOME)/.local

all: build

## build: build the UI and the static mdn binary
build: ui
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o mdn ./cmd/mdn

## ui-deps: install UI dependencies
ui-deps:
	pnpm --dir ui install --frozen-lockfile

## ui: produce ui/dist
ui: ui-deps
	pnpm --dir ui build
	touch ui/dist/.gitkeep

## test: run Go tests
test:
	go test ./...

## vet: static checks for Go and the UI
vet: ui-deps
	go vet ./...
	pnpm --dir ui typecheck

## check: everything CI runs
check: vet test build

## install: copy the binary to $(PREFIX)/bin (default ~/.local/bin)
install: build
	install -Dm755 mdn $(PREFIX)/bin/mdn

clean:
	rm -f mdn
	mkdir -p ui/dist
	find ui/dist -mindepth 1 ! -name .gitkeep -delete
	touch ui/dist/.gitkeep
