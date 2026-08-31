#!/usr/bin/env sh
# Authenticode-sign one freshly built binary, in place.
#
# Called as a GoReleaser post-build hook for every target, so the first thing it
# does is decide whether it applies at all. Putting that decision here rather
# than in a template is what makes it runnable -- and therefore testable --
# outside a release.
#
#   sign-windows.sh <path-to-binary> <goos>
#
# Signing is skipped, loudly, when WINDOWS_SIGN_P12 is unset. That is the
# ordinary state of a repository that has not bought a code-signing certificate,
# and a release must still complete there. What it must not do is complete
# silently: set REQUIRE_SIGNING=1 once the certificate exists and a missing one
# becomes a failed release rather than an unsigned download.
#
# Environment:
#   WINDOWS_SIGN_P12       base64 of a PKCS#12 code-signing certificate
#   WINDOWS_SIGN_PASSWORD  its passphrase (may be empty)
#   WINDOWS_SIGN_TIMESTAMP RFC3161 timestamp server
#                          (default: http://timestamp.digicert.com)
#   REQUIRE_SIGNING        1 to fail rather than skip
set -eu

binary="${1:?usage: sign-windows.sh <binary> <goos>}"
goos="${2:?usage: sign-windows.sh <binary> <goos>}"

say() { printf 'sign-windows: %s\n' "$1" >&2; }

if [ "$goos" != "windows" ]; then
	exit 0
fi

if [ -z "${WINDOWS_SIGN_P12:-}" ]; then
	if [ "${REQUIRE_SIGNING:-}" = "1" ]; then
		say "REQUIRE_SIGNING=1 but WINDOWS_SIGN_P12 is not set; refusing to ship an unsigned $binary"
		exit 1
	fi
	# Named on stderr so an unsigned release is a thing someone saw scroll past,
	# not a thing they have to go looking for.
	say "no WINDOWS_SIGN_P12; leaving $binary unsigned"
	exit 0
fi

if ! command -v osslsigncode >/dev/null 2>&1; then
	say "osslsigncode is not installed, but WINDOWS_SIGN_P12 is set"
	exit 1
fi

work="$(mktemp -d)"
# The certificate and passphrase reach disk here. Clear them on every exit
# path, including the signal that kills a cancelled workflow.
cleanup() { rm -rf "$work"; }
trap cleanup EXIT HUP INT TERM

printf '%s' "$WINDOWS_SIGN_P12" | base64 -d > "$work/cert.p12" 2>/dev/null || {
	say "WINDOWS_SIGN_P12 is not valid base64"
	exit 1
}
# Via a file, not an argument: an argument is visible in the process list to
# every other process on the runner.
printf '%s' "${WINDOWS_SIGN_PASSWORD:-}" > "$work/pass"

timestamp="${WINDOWS_SIGN_TIMESTAMP:-http://timestamp.digicert.com}"

say "signing $binary"
# Timestamping is not optional. An untimestamped signature stops validating the
# day the certificate expires, which turns every past release into a warning.
osslsigncode sign \
	-pkcs12 "$work/cert.p12" \
	-readpass "$work/pass" \
	-n "packetcode" \
	-i "https://github.com/packetloss404/packetcode" \
	-ts "$timestamp" \
	-h sha256 \
	-in "$binary" \
	-out "$work/signed.exe"

# Replace only after a clean signing run: a partial output overwriting the real
# binary would ship a corrupt file with a green pipeline.
mv "$work/signed.exe" "$binary"

# Verify what was actually produced rather than trusting the exit code above.
#
# This needs a CA bundle. `osslsigncode verify` with no -CAfile cannot build a
# chain from anything, so it fails for a perfectly good certificate as readily
# as for a bad one -- a check that always fails is not a check, and shipping one
# would print a chain warning on every legitimate release until people learned
# to ignore it. Where no bundle exists, say that the step was skipped instead of
# reporting a failure it did not establish.
ca=""
for candidate in \
	/etc/ssl/certs/ca-certificates.crt \
	/etc/pki/tls/certs/ca-bundle.crt \
	/etc/ssl/cert.pem; do
	if [ -r "$candidate" ]; then
		ca="$candidate"
		break
	fi
done

if [ -z "$ca" ]; then
	say "signed $binary (no CA bundle found; signature not verified here)"
	exit 0
fi

if osslsigncode verify -CAfile "$ca" -in "$binary" >"$work/verify.log" 2>&1; then
	say "signed $binary, signature verifies against $ca"
else
	# Not fatal. The signature is present and timestamped either way, and the
	# likeliest cause is a missing intermediate on this runner rather than a
	# bad signature -- but it is the operator's call, and they cannot make it
	# without being told.
	say "warning: $binary is signed but did not verify against $ca"
	sed 's/^/sign-windows:   /' "$work/verify.log" >&2
fi
