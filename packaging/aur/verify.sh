#!/usr/bin/env bash
# Build, lint, install and remove the rendered AUR package, inside a throwaway
# Arch container. .github/workflows/publish-aur.yml runs this and pushes
# nothing if it fails; a person can run the same thing:
#
#   podman run --rm -v "$PWD:/work:ro,Z" -w /work archlinux:latest \
#       packaging/aur/verify.sh v0.1.0
#
# (docker run, identically, without the :Z). Render first: it wants a PKGBUILD
# for a release that exists, because makepkg downloads that release's binaries
# and checks them against the checksums in it, so the committed 0.0.0/SKIP
# template fails with a 404.
#
#   gh release download v0.1.0 --pattern SHA256SUMS
#   go run ./scripts/aurgen -version v0.1.0 -sums SHA256SUMS
#
# What it proves, in order: that .SRCINFO is the one makepkg would write from
# this PKGBUILD — the AUR reads the package's version from .SRCINFO alone, and
# a stale one shows the wrong version forever; that the package builds; that
# namcap has nothing to say about either the PKGBUILD or the built package;
# that installing it puts the release's own binary on PATH, the tagged tree's
# unit — byte for byte, and one systemd reads without complaint — where
# systemd --user looks for it, and the licence where pacman expects it; and
# that removing it leaves nothing behind.
set -euo pipefail

version=${1:?the release tag the package must report, e.g. v0.1.0}
packaging=${2:-packaging/aur}
unit_source=${3:-contrib/mdn.service}

# This installs packages, installs the built package and removes it again. In a
# container that is a clean room; on a real machine it is somebody's system,
# and the AUR install M8-R3 asks for is done from the published package by
# hand, not by this script.
if [ ! -e /run/.containerenv ] && [ ! -e /.dockerenv ] && [ -z "${container:-}" ]; then
	echo "verify.sh installs and removes packages: run it in a container, not on a machine you use" >&2
	exit 1
fi

pacman -Syu --needed --noconfirm base-devel namcap sudo

# makepkg refuses to run as root, and rightly: the build is somebody else's
# code. It needs sudo only to install the package's own dependencies.
useradd --create-home builder
printf 'builder ALL=(ALL) NOPASSWD: ALL\n' >/etc/sudoers.d/builder
build=/home/builder/build
install -d -o builder -g builder "$build"
install -o builder -g builder -m644 \
	"$packaging/PKGBUILD" "$packaging/.SRCINFO" "$packaging"/*.install "$build/"

sudo -u builder --login bash -euo pipefail -s <<'BUILD'
cd ~/build

echo "== .SRCINFO is what makepkg writes from this PKGBUILD =="
makepkg --printsrcinfo >.SRCINFO.regenerated
diff -u .SRCINFO .SRCINFO.regenerated
rm .SRCINFO.regenerated

echo "== makepkg -s =="
makepkg --syncdeps --noconfirm

echo "== namcap =="
# namcap reports and exits 0 whatever it finds, so the verdict is the output:
# an E: line fails this, a W: line is for a human to read and either fix or
# justify on the pull request.
namcap PKGBUILD | tee namcap.out
namcap ./*.pkg.tar.zst | tee -a namcap.out
if grep -E ' E: ' namcap.out; then
	echo "namcap reported an error above" >&2
	exit 1
fi
BUILD

package=$(echo "$build"/*.pkg.tar.zst)
echo "== pacman -U $(basename "$package") =="
pacman -U --noconfirm "$package"

echo "== the installed package =="
installed=$(mdn version)
if [ "$installed" != "$version" ]; then
	echo "the installed binary reports $installed, want $version" >&2
	exit 1
fi
echo "mdn version: $installed"

unit=/usr/lib/systemd/user/mdn.service
# The package installs the unit verbatim (davison/md-notes#137), so the test is
# equality with the file in the tagged tree — which is what makepkg downloaded
# and checked the sha256 of, and what this checkout holds. Byte for byte says
# more than a grep for one line: a header that drifted, a directive dropped in
# packaging, a stray newline would all show here.
if ! cmp -s "$unit" "$unit_source"; then
	echo "$unit is not $unit_source:" >&2
	diff -u "$unit_source" "$unit" >&2 || true
	exit 1
fi
test -s /usr/share/licenses/md-notes-bin/LICENSE
# systemd's own reading of the unit. The comparison above says the package did
# not change the file; this says the file is one systemd accepts — a bad
# directive, a missing [Install], an ExecStart pointing nowhere. In a container
# the only obstacle is the runtime directory: without XDG_RUNTIME_DIR it fails
# with "Failed to lookup RuntimeDirectory path" and never looks at the file;
# with it set, it reads the unit and says nothing.
XDG_RUNTIME_DIR=/run systemd-analyze --user verify "$unit"
pacman -Qi md-notes-bin | grep -E '^(Name|Version|Depends On|Optional Deps|Provides|Conflicts With)'

echo "== pacman -Rns =="
pacman -Rns --noconfirm md-notes-bin
for path in /usr/bin/mdn "$unit" /usr/share/licenses/md-notes-bin; do
	if [ -e "$path" ]; then
		echo "$path survived the removal" >&2
		exit 1
	fi
done

echo "verified $version"
