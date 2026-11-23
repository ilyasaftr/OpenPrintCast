#!/usr/bin/env bash
# Downloads the sample documents that the CUPS ipptool suites (ipp-1.1.test,
# ipp-2.0.test, ipp-everywhere.test and the other test files the Go tests
# run) send, into a directory the tests search before ipptool's own data
# directory (/usr/share/cups/ipptool). No root is needed.
#
# Usage: scripts/fetch-cups-testdata.sh [DIRECTORY]
#
# DIRECTORY defaults to internal/net/testdata/cups-cache, which git ignores.
# The tests read the same directory, or the one named by the
# CUPS_TESTDATA_DIR environment variable. The script is idempotent: files
# whose SHA-256 already matches are not downloaded again.
#
# Sources, all pinned by content hash:
#  - the CUPS v2.4.7 documents, from the examples/ directory of commit
#    3a4d9204e5818e5d0b8f1e0e50832661209048ee. ipptool 2.4.7 does not ship
#    them (Ubuntu's cups-ipp-utils installs only the *.test files);
#  - the PWG Raster samples at 300 dpi, from the PWG FTP server. ipp-everywhere
#    .test prints samples at six resolutions, but each step is skipped
#    unless the printer advertises that resolution in
#    pwg-raster-document-resolution-supported, and the bridge advertises
#    300 dpi only, so no other resolution is downloaded (about 200 MB more).
#
# ipptool 2.4.7 checks that the file of every FILE line can be read while it
# parses the line, even in a step it skips, and aborts the run when one is
# missing (SKIP-IF-MISSING does not prevent that). For every sample that
# ipp-everywhere.test names and that was not downloaded, the script creates an
# empty placeholder file. A skipped step never reads its file; a step that
# starts running because the bridge now advertises another resolution fails
# on the empty file instead of passing, so the placeholders cannot hide a
# regression. The names come from ipp-everywhere.test of the pinned commit.
#
# The 2018 sample set names its photographs color-4x6-... and gray-4x6-...,
# while ipp-everywhere.test of CUPS 2.4.7 asks for color.jpg-4x6-... and
# gray.jpg-4x6-... (the names of the 2011 and 2015 sets, which the PWG moved
# to old-samples-do-not-use/). The script adds the names the suite uses as
# links to the files of the current set.
set -euo pipefail

CUPS_COMMIT=3a4d9204e5818e5d0b8f1e0e50832661209048ee
CUPS_RAW=https://raw.githubusercontent.com/OpenPrinting/cups/${CUPS_COMMIT}/examples

RASTER_NAME=pwg-raster-samples-300dpi
RASTER_URL=https://ftp.pwg.org/pub/pwg/ipp/examples/${RASTER_NAME}-20180607.zip
RASTER_SHA256=61ed106cf6e14d99ffb822ebd183684f48cc1c54ea179ba08829178dc7dc750c
RASTER_STAMP=.sha256

EVERYWHERE_TEST=ipp-everywhere.test
EVERYWHERE_TEST_SHA256=54dbc5c4b2c32fa835a811bdf7da30ed395e14a581f50aa432874f62c571a2b1

# "sha256  name" lines of the files taken from the CUPS repository.
CUPS_FILES="
b65d3a9a5898d82426455c0ec267894b37d7571599652d90e7048ba2bda6401b  onepage-a4.pdf
0415925d6db0f2b9c4e8c3fb72b04da9a524471604ccac7077033521d97e4c28  document-a4.pdf
13e32b5f7d67c34476c5793f8bcb4b47d5f40d3e19c1636f1e7aaa6206cf3f95  document-letter.pdf
10f21d9666fb551b165a2a3ea013e41b04918fd1493f63789908a748b459b97e  document-a4.ps
58cb1f64fa07848e6387ba45f5fcba15ce6f5d5c6c0bbc55d83c3dadabc045b2  document-letter.ps
fb858bad5febad17bd75631b951279628bfad3e88bff93d93d45e53f72c9b110  color.jpg
7f374fce68e05736df9ebe938a2dca8782f994897921c0c3e3ab610641deb014  gray.jpg
"

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
dest=${1:-$script_dir/../internal/net/testdata/cups-cache}
mkdir -p "$dest"
dest=$(cd "$dest" && pwd)

tmp=$(mktemp -d "$dest/.fetch.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

# has_sha256 FILE SHA256 succeeds when FILE exists and has that hash.
has_sha256() {
	[ -f "$1" ] && [ "$(sha256sum "$1" | cut -d' ' -f1)" = "$2" ]
}

# download URL FILE SHA256 fetches URL to FILE unless FILE already matches.
download() {
	local url=$1 file=$2 sha=$3
	if has_sha256 "$file" "$sha"; then
		return
	fi
	echo "fetching $(basename "$file")"
	curl -fsSL --retry 3 -o "$tmp/download" "$url"
	if ! has_sha256 "$tmp/download" "$sha"; then
		echo "error: $url does not have SHA-256 $sha" >&2
		exit 1
	fi
	mv "$tmp/download" "$file"
}

fetch_cups_files() {
	local sha name
	while read -r sha name; do
		[ -n "$name" ] || continue
		download "$CUPS_RAW/$name" "$dest/$name" "$sha"
	done <<<"$CUPS_FILES"
}

# link_suite_names adds color.jpg-4x6-... and gray.jpg-4x6-... links next to
# the color-4x6-... and gray-4x6-... files of the 2018 sample set.
link_suite_names() {
	local file base
	for file in "$dest/$RASTER_NAME"/*/{color,gray}-4x6-*.pwg; do
		[ -e "$file" ] || continue
		base=$(basename "$file")
		ln -sf "$base" "$(dirname "$file")/${base/-4x6-/.jpg-4x6-}"
	done
}

fetch_raster_samples() {
	local dir=$dest/$RASTER_NAME
	if [ "$(cat "$dir/$RASTER_STAMP" 2>/dev/null || true)" = "$RASTER_SHA256" ]; then
		return
	fi
	download "$RASTER_URL" "$tmp/raster.zip" "$RASTER_SHA256"
	echo "extracting $RASTER_NAME"
	unzip -q -o "$tmp/raster.zip" -d "$tmp/raster"
	rm -rf "$dir"
	mv "$tmp/raster/$RASTER_NAME" "$dir"
	link_suite_names
	printf '%s\n' "$RASTER_SHA256" >"$dir/$RASTER_STAMP"
}

# create_placeholders adds an empty file for each sample ipp-everywhere.test
# names that does not exist yet.
create_placeholders() {
	local list=$dest/.$EVERYWHERE_TEST rel
	download "$CUPS_RAW/$EVERYWHERE_TEST" "$list" "$EVERYWHERE_TEST_SHA256"
	while read -r rel; do
		if [ ! -e "$dest/$rel" ]; then
			mkdir -p "$dest/$(dirname "$rel")"
			: >"$dest/$rel"
		fi
	done < <(sed -n 's/^[[:space:]]*FILE \(pwg-raster-samples-[^[:space:]]*\)$/\1/p' "$list" | sort -u)
}

fetch_cups_files
fetch_raster_samples
create_placeholders
echo "CUPS test data is in $dest"
