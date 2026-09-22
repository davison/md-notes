VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Recursively expanded, both of them, so the two subprocesses behind them run
# only for the targets that use them — `build` and `release`. Immediately
# expanded (`:=`) they ran at parse time, on every invocation: `make install`
# and `make clean` forked `git describe` and `go env` for values neither of
# them reads, and under sudo that `go env` created /root/.config/go/telemetry
# (the review of PR #166, nit (b)).
LDFLAGS = -s -w -X main.version=$(VERSION)
EXTENSION_ZIP := extension/mdn-extension.zip
UNIT := contrib/mdn.service
MANPAGE := contrib/mdn.1
DIST ?= dist
# The release runs one of the binaries it just built to check what version
# it reports, so it builds on a host that can run one of its own targets.
HOST_ARCH = $(shell go env GOHOSTARCH)

.PHONY: all build man ui ui-deps extension extension-dist extension-deps test vet check e2e vuln release install clean distclean

# A system-wide install is the default, so that `make install` and the .deb
# put the binary in the same place and contrib/mdn.service points at one path
# rather than at whichever route the reader took (davison/md-notes#137).
# Set rather than left empty: an empty default would make a bare
# `make install` write to /bin, which is a symlink to /usr/bin on a merged-usr
# system and a different directory on anything else, and it would leave the
# unit's ExecStart true only by accident.
PREFIX ?= /usr

all: build

## build: build the UI, the static mdn binary and its manual page
build: ui man
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o mdn ./cmd/mdn

## man: write ./mdn.1, contrib/mdn.1 with its version and date filled in
# The version is VERSION without its v, as the .deb and the AUR package write
# it. The date is SOURCE_DATE_EPOCH's when that is set, and otherwise the HEAD
# commit's rather than today's, so building the same commit twice writes the
# same page (davison/md-notes#168). Done here rather than in `install`, which
# forks nothing: under sudo, git would be reading the tree as root.
man:
	@date=$$(date -u -d "@$${SOURCE_DATE_EPOCH:-$$(git log -1 --format=%ct 2>/dev/null || date +%s)}" +%Y-%m-%d) && \
	sed -e 's/@VERSION@/$(patsubst v%,%,$(VERSION))/g' -e "s/@DATE@/$$date/g" $(MANPAGE) > mdn.1 && \
	! grep -q '@[A-Z]*@' mdn.1 || { echo 'make man: a placeholder was left unfilled in mdn.1' >&2; exit 1; }

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
# Without the download each browser suite skips, with one line naming that
# command, and exits 0 — except under CI (`CI` set, and not `false` or `0`),
# where a missing prerequisite fails the run instead, so CI's e2e job can
# never go green by skipping (davison/md-notes#153).
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
# The whole dependency set, not --prod. The tempting line is that only what
# ships can hurt a user, but ui/dist is what the daemon serves and vite
# builds it: an advisory in the toolchain that produces the artefact reaches
# the artefact without ever appearing in a production dependency. The dev
# tree is also where a supply-chain compromise lands first, and it runs on
# the machine of everyone who builds this. Both workspaces audit clean with
# dev dependencies included, so the wider scan costs nothing today.
GOVULNCHECK ?= golang.org/x/vuln/cmd/govulncheck@v1.8.0

vuln:
	go run $(GOVULNCHECK) ./...
	pnpm --dir ui audit
	pnpm --dir extension audit

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

## install: copy the built ./mdn, its manual page and the unit into $(DESTDIR)$(PREFIX) (default /usr)
# Installs and nothing else. It has no prerequisite — deliberately: while it
# depended on `build`, `sudo make install` re-ran `pnpm install` and `go build`
# as root, with root's empty caches, downloading the toolchain again and
# leaving root-owned files in the checkout (davison/md-notes#163). Build as
# yourself, then install:
#
#     make build
#     sudo make install
#
# It refuses, having copied nothing, when any payload is missing. The three
# destinations are paths the .deb installs (packaging/deb/nfpm.yaml), so the
# from-source route and the package agree on the unit and the manual page as
# well as on the binary, and the unit goes in byte for byte either way. The
# page is the one `make build` wrote, gzipped as the .deb ships it, so
# `man mdn` works whichever way mdn was installed (davison/md-notes#168).
#
# DESTDIR relocates both for a staged install, which is also how to try the
# whole thing without root:
#
#     make install DESTDIR=$$PWD/dist/scratch
#
# PREFIX=$$HOME/.local restores the old per-user install and needs no root;
# contrib/mdn.service then wants a drop-in pointing ExecStart at it, which its
# header spells out.
#
# It prints the two `systemctl --user` lines rather than running them: under
# sudo, `systemctl --user` is root's session, not the session a user unit has
# to run in, so enabling it is the invoking user's step and saying so at the
# terminal is the most this target can honestly do (davison/md-notes#164).
# A staged install says it staged instead: nothing under DESTDIR is where
# systemd looks, so those two lines would either do nothing or enable whatever
# was installed for real earlier.
install:
	@test -f mdn || { echo 'make install: ./mdn is not here — run `make build` first; install does not build.' >&2; exit 1; }
	@test -f mdn.1 || { echo 'make install: ./mdn.1 is not here — run `make build` first; install does not build.' >&2; exit 1; }
	@test -f $(UNIT) || { echo 'make install: $(UNIT) is not here — run make install from the repository root.' >&2; exit 1; }
	install -Dm755 mdn $(DESTDIR)$(PREFIX)/bin/mdn
	install -Dm644 $(UNIT) $(DESTDIR)$(PREFIX)/lib/systemd/user/mdn.service
	install -Dm644 mdn.1 $(DESTDIR)$(PREFIX)/share/man/man1/mdn.1
	gzip -9nf $(DESTDIR)$(PREFIX)/share/man/man1/mdn.1
	@echo
	@echo 'Installed $(DESTDIR)$(PREFIX)/bin/mdn'
	@echo '          $(DESTDIR)$(PREFIX)/share/man/man1/mdn.1.gz'
	@echo '      and $(DESTDIR)$(PREFIX)/lib/systemd/user/mdn.service'
	@echo
ifeq ($(strip $(DESTDIR)),)
	@echo 'Now, as the user who will run the daemon (not root):'
	@echo
	@echo '    systemctl --user daemon-reload'
	@echo '    systemctl --user enable --now mdn'
else
	@echo 'Staged under $(DESTDIR): nothing is installed on this system, so'
	@echo 'there is nothing to enable yet.'
endif

# What `build`, `extension` and `release` write, and nothing else. ui/dist is
# emptied rather than removed: its .gitkeep is tracked.
CLEAN_PATHS := mdn mdn.1 $(EXTENSION_ZIP) $(DIST) extension/dist

## clean: remove what build, extension and release produce
# Every path is attempted and whatever is left is named at the end, rather than
# the first `rm` failure stopping the rest: a tree with root-owned leftovers in
# it — from a `sudo make` of a version older than davison/md-notes#164 — should
# tell the operator everything they have to go and remove, in one run. It still
# exits nonzero, because the tree is not clean.
#
# node_modules is distclean's: it is a lockfile-keyed cache of other people's
# code, minutes and a network to rebuild, and `clean && check` has to stay a
# thing you can do offline (the decision is on davison/md-notes#164).
clean:
	@left=''; \
	for p in $(CLEAN_PATHS); do \
		rm -rf "$$p" 2>/dev/null; \
		if [ -e "$$p" ]; then left="$$left $$p"; fi; \
	done; \
	if [ -d ui/dist ]; then \
		find ui/dist -mindepth 1 -maxdepth 1 ! -name .gitkeep -exec rm -rf {} + 2>/dev/null; \
		if [ -n "$$(find ui/dist -mindepth 1 -maxdepth 1 ! -name .gitkeep -print -quit 2>/dev/null)" ]; then left="$$left ui/dist"; fi; \
	fi; \
	mkdir -p ui/dist 2>/dev/null; touch ui/dist/.gitkeep 2>/dev/null; \
	if [ -n "$$left" ]; then \
		echo 'make clean: could not remove:' >&2; \
		for p in $$left; do echo "  $$p" >&2; done; \
		echo 'Root-owned, from a `sudo make` of an older tree? Remove them as root — `make install` no longer builds anything, so nothing here will be root-owned again.' >&2; \
		exit 1; \
	fi

## distclean: clean, and the two pnpm dependency trees as well
# The next build then re-runs `pnpm install` for both workspaces, which needs
# the network.
#
# It goes through `clean`, so a tree `clean` could not finish stops here with
# its report and keeps its node_modules: a tree you cannot clean is not one to
# delete more of, and the leftovers it named are what to deal with first.
distclean: clean
	rm -rf ui/node_modules extension/node_modules
