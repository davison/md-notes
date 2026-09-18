#!/usr/bin/env bash
# The half of packaging/deb/verify.sh that runs inside the container
# (davison/md-notes#137). Not run directly: verify.sh feeds it to `bash -s`
# inside each image, with /debs and /packaging mounted and the container's
# Debian architecture as its one argument.

set -euo pipefail

readonly NATIVE="${1:?the Debian architecture of this container}"
export DEBIAN_FRONTEND=noninteractive

say() { echo; echo "-- $*"; }
die() {
	echo "verify: $*" >&2
	exit 1
}

. /etc/os-release
say "$PRETTY_NAME, $(dpkg --print-architecture)"
[ "$(dpkg --print-architecture)" = "$NATIVE" ] ||
	die "the image is $(dpkg --print-architecture) but the host asked for $NATIVE"

apt-get update -qq >/dev/null
apt-get install -y -qq lintian systemd >/dev/null
lintian --version

# ---------------------------------------------------------------- lintian
# Both packages, whatever this container can run: lintian reads a .deb, it
# does not execute it.
for deb in /debs/md-notes_*.deb; do
	say "lintian $(basename "$deb")"
	# No exit status echoed after it: `set -e` is what checks it, so a line
	# printing $? here could only ever print 0 and would read as evidence it
	# is not. An overridden tag prints as O: and does not fail the run.
	lintian --display-info --show-overrides --tag-display-limit 0 "$deb"
done

# ------------------------------------------------------------ the packages
native=(/debs/md-notes_*_"$NATIVE".deb)
[ -f "${native[0]}" ] || die "no $NATIVE package in /debs"
deb="${native[0]}"
version=$(basename "$deb" | sed -E 's/^md-notes_(.+)_[a-z0-9]+\.deb$/\1/')

# ------------------------------------------------------- the foreign package
# The other architecture cannot be installed here, so it is read rather than
# run: the control fields and the file list are checked to be the same package
# by another name.
for foreign in /debs/md-notes_*.deb; do
	[ "$foreign" = "$deb" ] && continue
	say "$(basename "$foreign") is not this container's architecture: structure only"
	dpkg-deb --field "$foreign" Package Version Architecture Depends Section Priority Maintainer Homepage
	dpkg-deb --contents "$foreign" | awk '$1 !~ /^d/ {print "   " $6}'
done

say "before: is ripgrep installed?"
if dpkg-query -W -f='${Status}' ripgrep 2>/dev/null | grep -q "^install ok installed$"; then
	die "ripgrep is already installed in this image, so installing the package would prove nothing"
fi
echo "   no — good, so what follows is the package's own dependency"

# The literal form, run from the directory the packages are in, because the
# label above this line is quoted as a transcript and should therefore be the
# command that ran. `apt install ./name.deb` and `apt install /path/name.deb`
# behave identically; only one of them is what the pull request shows.
say "apt install ./$(basename "$deb")"
(cd /debs && apt-get install -y "./$(basename "$deb")")

say "after: ripgrep came in with it"
dpkg-query -W -f='ripgrep ${Version} ${Status}\n' ripgrep
command -v rg >/dev/null || die "ripgrep is marked installed but rg is not on PATH"

say "mdn version"
reported=$(mdn version)
echo "   $reported"
[ "$reported" = "v$version" ] ||
	die "the package is version $version but the binary reports $reported"

say "systemd-analyze --user verify /usr/lib/systemd/user/mdn.service"
export XDG_RUNTIME_DIR=/run/user/0
mkdir -p "$XDG_RUNTIME_DIR"
grep '^ExecStart=' /usr/lib/systemd/user/mdn.service
# It grumbles about the system bus, which a container has not got; what is
# being checked is the exit status, and `set -e` is what checks it.
systemd-analyze --user verify /usr/lib/systemd/user/mdn.service
echo "   accepted: exit 0, no complaint about the unit itself"

say "the files the package owns"
mapfile -t owned < <(dpkg -L md-notes)
printf '   %s\n' "${owned[@]}"

# -------------------------------------------------------------- the removal
say "apt remove md-notes"
apt-get remove -y md-notes

say "nothing of the package is left"
left=()
for path in "${owned[@]}"; do
	# Directories the package merely populated — /usr/bin, /usr/share/man —
	# belong to the distribution and stay. Its own directory does not.
	if [ -d "$path" ] && [ "$path" != /usr/share/doc/md-notes ]; then
		continue
	fi
	[ -e "$path" ] && left+=("$path")
done
if [ ${#left[@]} -gt 0 ]; then
	printf '   still there: %s\n' "${left[@]}"
	die "apt remove left ${#left[@]} path(s) behind"
fi
echo "   every file and the package's own directory are gone"
dpkg-query -W -f='   dpkg still knows: ${Package} ${Status}\n' md-notes 2>/dev/null ||
	echo "   dpkg no longer knows the package"

say "$PRETTY_NAME/$NATIVE passed"
