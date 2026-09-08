#!/usr/bin/env sh
# Report presence only: certificate contents must never reach the job summary.
set -eu

macos="no — MACOS_SIGN_P12 is not set"
windows="no — WINDOWS_SIGN_P12 is not set"
if [ -n "${MACOS_SIGN_P12:-}" ]; then
	macos="yes"
fi
if [ -n "${WINDOWS_SIGN_P12:-}" ]; then
	windows="yes"
fi

printf '%s\n' \
	'### Signing configuration' \
	'' \
	'| artifact | signed |' \
	'| --- | --- |' \
	'| checksums.txt (Sigstore) | yes — keyless, always available in this workflow |' \
	"| macOS binaries (Developer ID + notarization) | $macos |" \
	"| Windows binaries (Authenticode) | $windows |" \
	'' \
	"REQUIRE_SIGNING=${REQUIRE_SIGNING:-0}. Set the repository variable to 1 once certificates exist, and a missing one fails the release instead of shipping unsigned."
