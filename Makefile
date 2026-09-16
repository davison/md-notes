VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
EXTENSION_ZIP := extension/mdn-extension.zip

.PHONY: all build ui ui-deps extension extension-dist extension-deps test vet check e2e install clean
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

## extension-deps: install browser extension dependencies
extension-deps:
	pnpm --dir extension install --frozen-lockfile

## extension-dist: produce extension/dist, loadable unpacked
extension-dist: extension-deps
	pnpm --dir extension build

## extension: extension/dist and the zip beside it
extension: extension-dist
	pnpm --dir extension run zip

## test: run Go, UI and extension tests
# Depends on ui, not ui-deps: TestUITypesCoverTheBundle checks the daemon's
# Content-Type table against the build's actual output, and skips when dist
# is empty.
test: ui extension-deps
	go test ./...
	pnpm --dir ui test
	pnpm --dir extension test

## vet: static checks for Go, the UI and the extension
vet: ui-deps extension-deps
	go vet ./...
	pnpm --dir ui typecheck
	pnpm --dir extension typecheck

## check: everything CI runs
check: vet test build extension

## e2e: browser-level checks for the UI, in headless Chromium against the built daemon
# Not part of `check`: it needs Playwright's Chromium, which `make ui-deps`
# installs the driver for but does not download. Fetch it once with
#
#     pnpm --dir ui exec playwright install chromium
#
# The suite skips rather than fails when the browser or the binary is absent.
e2e: build
	pnpm --dir ui e2e

## install: copy the binary to $(PREFIX)/bin (default ~/.local/bin)
install: build
	install -Dm755 mdn $(PREFIX)/bin/mdn

clean:
	rm -f mdn $(EXTENSION_ZIP)
	mkdir -p ui/dist
	find ui/dist -mindepth 1 ! -name .gitkeep -delete
	touch ui/dist/.gitkeep
	rm -rf extension/dist
