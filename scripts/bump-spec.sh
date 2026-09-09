#!/bin/sh
# Set the version in packaging/shanty.spec and add a changelog stanza.
#
# The spec is the one place a version is written. The tag is derived from it by
# `make release-tag`, and the release workflow refuses a tag that disagrees.
set -eu

version="${1:?usage: bump-spec.sh X.Y.Z}"
spec="packaging/shanty.spec"
name="Bryan Speelman"
email="bryspeelm@gmail.com"

echo "$version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' || {
	echo "version must be X.Y.Z, not $version" >&2
	exit 1
}
test -f "$spec" || { echo "no $spec" >&2; exit 1; }

# C locale: rpm wants an English day and month whatever the machine speaks.
date=$(LC_ALL=C date '+%a %b %d %Y')
tmp=$(mktemp)

sed "s/^Version:.*/Version:        $version/" "$spec" > "$tmp"
awk -v entry="* $date $name <$email> - $version-1" \
    -v url="- See https://github.com/bspeelm/shanty/releases/tag/v$version" '
    { print }
    /^%changelog$/ { print entry; print url; print "" }
' "$tmp" > "$spec"
rm -f "$tmp"

echo "packaging/shanty.spec is now $version"
