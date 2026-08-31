#!/usr/bin/env sh
# Assert that a built dist/ is a complete, self-consistent release.
#
#   check-release-artifacts.sh [dist-dir]
#
# This exists because the release pipeline was previously first exercised by
# tagging. `goreleaser check` validates the configuration file and nothing else;
# a snapshot build that merely exits 0 proves the tool ran, not that it produced
# six usable archives whose checksums are right and whose contents include the
# licence. The gap between those is where a release breaks, on the one day it is
# most expensive to find out.
#
# So CI now builds a snapshot on every push and runs this against it. Every
# assertion below corresponds to something a user does with the artifacts.
set -eu

dist="${1:-dist}"
fail=0

ok() { printf '  ok   %s\n' "$1"; }
bad() {
	printf '  FAIL %s\n' "$1"
	fail=1
}

printf 'checking %s\n' "$dist"

if [ ! -d "$dist" ]; then
	printf 'check-release-artifacts: %s does not exist; run goreleaser first\n' "$dist" >&2
	exit 1
fi

# Every platform the project claims to support, spelled exactly as the install
# scripts construct the name. A mismatch here is the failure where install.sh
# 404s for one architecture and nobody notices until somebody on it complains.
expected="packetcode-linux-amd64.tar.gz
packetcode-linux-arm64.tar.gz
packetcode-darwin-amd64.tar.gz
packetcode-darwin-arm64.tar.gz
packetcode-windows-amd64.zip
packetcode-windows-arm64.zip"

for archive in $expected; do
	if [ -f "$dist/$archive" ]; then
		ok "$archive present"
	else
		bad "$archive is missing"
	fi
done

if [ ! -f "$dist/checksums.txt" ]; then
	bad "checksums.txt is missing"
	printf '\ncheck-release-artifacts: FAILED\n' >&2
	exit 1
fi

# The checksums have to be right, not merely present. This is the file both
# installers trust, so a stale or partial one is worse than none.
if (
	cd "$dist" || exit 1
	if sha256sum --check --quiet checksums.txt 2>/dev/null; then
		exit 0
	fi
	exit 1
); then
	ok "checksums.txt verifies against the archives"
else
	bad "checksums.txt does not match the archives"
fi

# Every archive must be listed. A checksum file that silently omits one is the
# case where verification passes and the download was never checked at all,
# because `sha256sum --ignore-missing` has nothing to compare.
for archive in $expected; do
	if grep -q " \*\{0,1\}$archive\$" "$dist/checksums.txt"; then
		ok "$archive is listed in checksums.txt"
	else
		bad "$archive is not listed in checksums.txt"
	fi
done

# The binary has to actually be in the archive, under the name the installers
# extract, along with the licence that has to travel with it.
for archive in $expected; do
	[ -f "$dist/$archive" ] || continue
	# A listing that fails is a finding, not a reason to abort. Under `set -e`
	# an unreadable archive killed the script mid-report, so the checks after
	# it never ran and the summary never printed -- the run said less about the
	# release than it had already discovered.
	case "$archive" in
	*.tar.gz) listing="$(tar -tzf "$dist/$archive" 2>/dev/null)" || listing="" ;;
	*.zip)
		if command -v unzip >/dev/null 2>&1; then
			listing="$(unzip -Z1 "$dist/$archive" 2>/dev/null)" || listing=""
		else
			printf '  skip %s (unzip not installed)\n' "$archive"
			continue
		fi
		;;
	*) continue ;;
	esac

	if [ -z "$listing" ]; then
		bad "$archive could not be listed; it is corrupt or truncated"
		continue
	fi

	case "$archive" in
	*windows*) binary="packetcode.exe" ;;
	*) binary="packetcode" ;;
	esac

	# Spelled out rather than `A && ok || bad`: in that idiom the failure
	# branch also runs when the success branch returns non-zero, and a check
	# script that can report both outcomes for one condition is worse than
	# no check at all.
	if printf '%s\n' "$listing" | grep -qx "$binary"; then
		ok "$archive contains $binary"
	else
		bad "$archive does not contain $binary"
	fi
	if printf '%s\n' "$listing" | grep -qx "LICENSE"; then
		ok "$archive carries LICENSE"
	else
		bad "$archive does not carry LICENSE"
	fi
done

# Signatures are reported, never required: a dry run and a fork have no OIDC
# token, and failing them for that would make this script unusable in the place
# it runs most. The release workflow enforces signing with REQUIRE_SIGNING.
if [ -f "$dist/checksums.txt.sig" ] && [ -f "$dist/checksums.txt.pem" ]; then
	ok "checksums.txt is signed"
else
	printf '  note checksums.txt is unsigned (expected outside a tagged release)\n'
fi

if [ "$fail" -ne 0 ]; then
	printf '\ncheck-release-artifacts: FAILED\n' >&2
	exit 1
fi
printf '\ncheck-release-artifacts: all checks passed\n'
