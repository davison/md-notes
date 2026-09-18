VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
EXTENSION_ZIP := extension/mdn-extension.zip
DIST ?= dist
# The release runs one of the binaries it just built to check what version
# it reports, so it builds on a host that can run one of its own targets.
HOST_ARCH := $(shell go env GOHOSTARCH)

.PHONY: all build ui ui-deps extension extension-dist extension-deps test vet check e2e vuln release install clean
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
# The script names the files by glob rather than passing the directory:
# `node --test e2e/` is a module path to Node 24, not a directory to walk.
e2e: build
	pnpm --dir ui e2e

## vuln: scan the Go module graph and both lockfiles for published vulnerabilities
# Not part of `check`, and deliberately: govulncheck downloads the
# vulnerability database and `pnpm audit` asks the registry, so folding the
# scan into `check` would make the local edit-and-check loop fail whenever the
# network is away, and would put a check whose answer changes without the tree
# changing in front of every build. CI runs this as its own step of the
# `check` job instead (davison/md-notes#147) — the same job release.yml calls
# through workflow_call, so a release is scanned on the same terms.
#
# Needs no `ui-deps`/`extension-deps`: pnpm audit reads the lockfile, not
# node_modules.
#
# The scanner is pinned while the data it reads is fetched at run time and so
# is always current; nothing is gained by letting the tool itself float.
# govulncheck exits 3 and pnpm audit exits 1 on a finding, so either one fails
# the build.
#
# --prod because the question is what a user runs: the daemon, the bundle it
# serves and the extension zip. An advisory against vite or vitest is worth
# knowing and is not worth failing an unrelated pull request over.
GOVULNCHECK ?= golang.org/x/vuln/cmd/govulncheck@v1.8.0

vuln:
	go run $(GOVULNCHECK) ./...
	pnpm --dir ui audit --prod
	pnpm --dir extension audit --prod

## release: build dist/ for VERSION: both binaries, the extension zip and SHA256SUMS
# Everything the release workflow publishes, built here rather than in the
# workflow, so a release can be proven locally without pushing a tag:
#
#     make release VERSION=v0.1.0
#
# The version reaches the binaries through -ldflags and the extension manifest
# through MDN_VERSION, both from VERSION; relcheck then refuses to let the
# release go out if they do not agree.
release: ui extension-deps
	rm -rf $(DIST)
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/mdn-$(VERSION)-linux-amd64 ./cmd/mdn
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/mdn-$(VERSION)-linux-arm64 ./cmd/mdn
	MDN_VERSION=$(VERSION) pnpm --dir extension build
	pnpm --dir extension run zip
	cp $(EXTENSION_ZIP) $(DIST)/mdn-extension-$(VERSION).zip
	go run ./scripts/relcheck -version '$(VERSION)' -binary $(DIST)/mdn-$(VERSION)-linux-$(HOST_ARCH) -manifest $(DIST)/mdn-extension-$(VERSION).zip
	cd $(DIST) && sha256sum mdn-* > SHA256SUMS

## install: copy the binary to $(PREFIX)/bin (default ~/.local/bin)
install: build
	install -Dm755 mdn $(PREFIX)/bin/mdn

clean:
	rm -f mdn $(EXTENSION_ZIP)
	rm -rf $(DIST)
	mkdir -p ui/dist
	find ui/dist -mindepth 1 ! -name .gitkeep -delete
	touch ui/dist/.gitkeep
	rm -rf extension/dist
