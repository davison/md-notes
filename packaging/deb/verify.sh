#!/usr/bin/env bash
# Verify the md-notes .deb packages in the containers M8-R4 names
# (davison/md-notes#137).
#
#     packaging/deb/verify.sh <deb-dir> [image ...]
#
# Default images: debian:stable and ubuntu:24.04. For each one it runs lintian
# over both packages, then the whole life of the package in that distribution:
#
#   * `apt install ./md-notes_<version>_<arch>.deb` pulls ripgrep in — checked
#     by seeing that ripgrep is absent before and present after, so it proves
#     the dependency rather than the image's contents;
#   * `mdn version` prints the version the file name carries;
#   * `systemd-analyze --user verify` accepts the installed unit;
#   * `apt remove` leaves no file of the package's behind — checked against
#     `dpkg -L`, the package's own list, not a guess at what it installed.
#
# A transcript rather than a summary: what it prints is what the pull request
# quotes. Exits non-zero at the first thing that is not so.
#
# The install cycle needs a container of the same architecture as the package,
# so on a machine without binfmt emulation registered the foreign architecture
# gets lintian and a structural check and says so. Nothing is skipped silently.
#
# Uses podman if it is there, otherwise docker.

set -euo pipefail

readonly HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly DEFAULT_IMAGES=(debian:stable ubuntu:24.04)

die() {
	echo "packaging/deb/verify.sh: $*" >&2
	exit 1
}

main() {
	[ $# -ge 1 ] || die "usage: verify.sh <deb-dir> [image ...]"

	local debs
	debs="$(cd -- "$1" 2>/dev/null && pwd)" || die "no such directory: $1"
	shift
	local images=("$@")
	[ ${#images[@]} -gt 0 ] || images=("${DEFAULT_IMAGES[@]}")

	local engine
	engine="$(command -v podman || command -v docker)" ||
		die "neither podman nor docker is installed, and the verification M8-R4 asks for runs in containers"

	compgen -G "$debs/md-notes_*_amd64.deb" >/dev/null || die "no amd64 package in $debs"
	compgen -G "$debs/md-notes_*_arm64.deb" >/dev/null || die "no arm64 package in $debs"

	local image
	for image in "${images[@]}"; do
		echo
		echo "==================================================================="
		echo "== $image"
		echo "==================================================================="
		# --arch, not the image's cached default: a machine that has pulled the
		# same tag for another architecture would otherwise run that one, and
		# the failure ("exec format error") looks nothing like its cause.
		"$engine" run --rm -i --arch "$(deb_host_arch)" \
			-v "$debs:/debs:ro" -v "$HERE:/packaging:ro" \
			"$image" bash -s -- "$(deb_host_arch)" <"$HERE/verify-inside.sh"
	done

	echo
	echo "packaging/deb/verify.sh: all images passed"
}

# deb_host_arch is this machine's Debian architecture name.
deb_host_arch() {
	case "$(uname -m)" in
	x86_64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) die "no Debian architecture known for $(uname -m)" ;;
	esac
}

main "$@"
