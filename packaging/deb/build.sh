#!/usr/bin/env bash
# Build the md-notes .deb packages for amd64 and arm64 (davison/md-notes#137).
#
#     packaging/deb/build.sh <version> <assets-dir> <output-dir>
#
# <version> is the release tag verbatim, leading v and all — v0.1.0 — because
# that is what the release's asset names carry (docs/releasing.md). The Debian
# version is the same string with the v stripped: a v is not part of a Debian
# version, and dpkg would read one as an upstream version sorting before every
# digit.
#
# <assets-dir> holds the release's binaries under their published names,
# mdn-<version>-linux-amd64 and mdn-<version>-linux-arm64 — either a local
# `make release` dist/ or the directory the publish workflow downloaded them
# into. The binaries are packaged as they are found rather than rebuilt, so
# what a Debian user installs is byte for byte what the release's SHA256SUMS
# attests to.
#
# The packages land in <output-dir> as md-notes_<debian-version>_<arch>.deb.
#
# nfpm does the packaging; set NFPM to use a particular binary, as the publish
# workflow does with the pinned one it downloads. Set SOURCE_DATE_EPOCH to fix
# the changelog's date — the workflow sets it to the moment the release was
# published, so the changelog says when the release happened rather than when
# the runner got round to it.
#
# It packages what it is given and vouches for nothing about it: the only test
# applied to a binary here is that the file exists. The publish workflow checks
# the assets against the release's SHA256SUMS before calling this, and a test
# pins that ordering; a run by hand against a directory nobody checked will
# package whatever is in it.
#
#     make release VERSION=v0.1.0
#     packaging/deb/build.sh v0.1.0 dist dist/deb
#
# An output directory under dist/, which .gitignore already covers: a run that
# writes 14 MB of packages into the working tree leaves them to be committed by
# accident.

set -euo pipefail

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd -- "$HERE/../.." && pwd)"
readonly HERE REPO
readonly ARCHS=(amd64 arm64)
readonly PACKAGE=md-notes
readonly MAINTAINER='Darren Davison <darren@davisononline.org>'
readonly HOMEPAGE='https://github.com/davison/md-notes'

# Set by main before it can fail with the directory half-built; the trap that
# removes it runs after main has returned, so it cannot be one of main's
# locals.
STAGING=

die() {
	echo "packaging/deb/build.sh: $*" >&2
	exit 1
}

cleanup() {
	[ -n "$STAGING" ] && rm -rf -- "$STAGING"
}

# debian_version is the tag with its leading v removed, once.
#
# The shape is exactly the release workflow's, `v<major>.<minor>.<patch>`, and
# deliberately no wider. A wider one would be a trap rather than a
# convenience: a version with a suffix — v0.1.0-rc1, or a `make release
# VERSION=v0.0.0-test` proving build — has a hyphen in it, dpkg reads
# everything after the last hyphen as a Debian revision, and the package
# silently stops being native. Its changelog would then have to be named
# changelog.Debian.gz instead of changelog.gz, so the same script would
# produce a package that fails lintian for a reason nothing here mentions.
# release.yml cannot build such a version anyway, so there is nothing to
# support. Prove this locally with `make release VERSION=v0.0.0`.
#
# Anything else is refused rather than passed on. This string reaches a file
# name, a control field and a command line, and dpkg's rules are narrower than
# a shell's: an underscore or a space in it makes a package dpkg declines to
# read, at the far end of a release where nobody is watching.
debian_version() {
	local tag="$1"
	if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
		die "not a release version: '$tag' (want v<major>.<minor>.<patch>)"
	fi
	printf '%s' "${tag#v}"
}

# stage_unit puts contrib/mdn.service in the staging directory.
#
# Unchanged, byte for byte. It used to be rewritten here: that file ran
# %h/.local/bin/mdn, which a package cannot, so the packaging substituted the
# path — the gate raised on davison/md-notes#137. The operator resolved that
# gate as option (b): `make install` now installs system-wide by default and
# contrib/mdn.service runs /usr/bin/mdn directly, so there is nothing left to
# substitute and the package ships the file as the repository holds it. A
# per-user install points the unit at its own binary with a drop-in, which the
# unit's header spells out.
#
# So the copy is the whole of it, and the check below is that the file really
# does run the path this package installs to — if contrib/mdn.service ever
# goes back to a home-directory path, the package would otherwise ship a unit
# that cannot start, which is exactly what the gate was about.
stage_unit() {
	local src="$REPO/contrib/mdn.service" dst="$1"
	grep -qx 'ExecStart=/usr/bin/mdn serve' "$src" ||
		die "$src does not run /usr/bin/mdn serve, which is where this package installs the binary"
	install -m 0644 "$src" "$dst"
}

# stage_changelog writes the Debian changelog Policy 12.7 asks for.
#
# One entry, naming the version and pointing at the release notes. This
# project's changelog is GitHub's generated notes, written from the commits and
# pull requests since the previous tag; copying them in here would be a second
# copy to keep in step, and a stale one the first time anyone edited the notes
# before publishing. The entry is what a Debian tool needs to answer "what
# version is this and who made it", and the link is where the content is.
stage_changelog() {
	local version="$1" dst="$2" date
	date="$(date -R -u ${SOURCE_DATE_EPOCH:+-d "@$SOURCE_DATE_EPOCH"})"
	cat >"$dst" <<-ENTRY
		$PACKAGE ($version) unstable; urgency=medium

		  * md-notes $version. The release notes for this version are at
		    https://github.com/davison/md-notes/releases/tag/v$version

		 -- $MAINTAINER  $date
	ENTRY
	# -9 because lintian asks for maximum compression, -n so the package does
	# not carry a build timestamp inside the gzip header as well as in it.
	gzip -9n "$dst"
}

# stage_copyright writes the copyright file Policy 12.5 asks for: the licence
# verbatim, under a line saying where the sources it covers came from.
#
# Not DEP-5, and deliberately: the machine-readable format exists to describe a
# package whose files carry several licences and several copyright holders, and
# this one is a single MIT-licensed tree. What Policy asks for that the licence
# text alone does not give is the upstream source, so that is the line that is
# added and nothing else — lintian is satisfied either way in both
# distributions, so this is for the person who reads the file.
stage_copyright() {
	local dst="$1"
	{
		echo "Source: $HOMEPAGE"
		echo
		cat "$REPO/LICENSE"
	} >"$dst"
}

# stage_manpage writes packaging/deb/mdn.1 with its placeholders filled in.
stage_manpage() {
	local version="$1" dst="$2" date
	date="$(date -u +%Y-%m-%d ${SOURCE_DATE_EPOCH:+-d "@$SOURCE_DATE_EPOCH"})"
	sed -e "s/@VERSION@/$version/g" -e "s/@DATE@/$date/g" "$HERE/mdn.1" >"$dst"
	! grep -q '@[A-Z]*@' "$dst" || die "a placeholder was left unfilled in $dst"
	gzip -9n "$dst"
}

main() {
	[ $# -eq 3 ] || die "usage: build.sh <version> <assets-dir> <output-dir>"

	local tag="$1" assets out version
	assets="$(cd -- "$2" 2>/dev/null && pwd)" || die "no such assets directory: $2"
	mkdir -p -- "$3"
	out="$(cd -- "$3" && pwd)"
	version="$(debian_version "$tag")"

	# nfpm resolves `contents[].src` against the working directory and does not
	# expand environment variables there, so everything the package carries is
	# staged under the fixed names nfpm.yaml lists and nfpm is run from the
	# staging directory.
	STAGING="$(mktemp -d)"
	trap cleanup EXIT
	stage_unit "$STAGING/mdn.service"
	stage_changelog "$version" "$STAGING/changelog"
	stage_manpage "$version" "$STAGING/mdn.1"
	stage_copyright "$STAGING/copyright"
	install -m 0644 "$HERE/lintian-overrides" "$STAGING/lintian-overrides"

	# Every asset before any package: the loop below writes one package per
	# architecture, so finding the second binary missing half way through
	# would leave the first package sitting in the output directory as though
	# the run had gone well.
	local arch binary
	for arch in "${ARCHS[@]}"; do
		binary="$assets/mdn-$tag-linux-$arch"
		[ -f "$binary" ] || die "no $arch binary at $binary"
	done

	local target
	for arch in "${ARCHS[@]}"; do
		binary="$assets/mdn-$tag-linux-$arch"
		# The staged copy is what nfpm reads, and a binary downloaded over
		# HTTP arrives without its executable bit.
		install -m 0755 "$binary" "$STAGING/mdn"

		target="$out/${PACKAGE}_${version}_${arch}.deb"
		(
			cd -- "$STAGING"
			DEB_ARCH="$arch" DEB_VERSION="$version" \
				"${NFPM:-nfpm}" package \
				--config "$HERE/nfpm.yaml" \
				--packager deb \
				--target "$target"
		)
		echo "packaging/deb/build.sh: built $target"
	done
}

main "$@"
