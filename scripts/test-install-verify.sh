#!/usr/bin/env bash
# Exercise install.sh's signature check without installing anything.
#
# This is the least testable code in the repository and among the most
# dangerous: it runs as `curl | bash` on machines that have no packetcode on
# them yet, and it has two failure modes that are invisible from reading it.
# Get the cosign invocation wrong and every install with cosign present dies at
# the last step. Get the branch structure wrong and a signature that does not
# verify is treated as one that was merely absent -- which is the whole attack,
# because an attacker who can substitute checksums.txt can also strip the
# signature beside it.
#
# So the function is lifted out of install.sh and run against stubbed curl and
# cosign. Stubs, rather than a live download, because the cases worth asserting
# are the ones GitHub will not serve on request: a forged signature, and a
# release that has none.
set -uo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
pass=0
fail=0

check() {
	if [ "$2" = "$3" ]; then
		printf '  ok   %s\n' "$1"
		pass=$((pass + 1))
	else
		printf '  FAIL %s: got %q, want %q\n' "$1" "$2" "$3"
		fail=$((fail + 1))
	fi
}

# Lift verify_signature out of install.sh rather than duplicating it. A copy
# would drift, and a test of a copy asserts nothing about what ships.
extract() {
	sed -n '/^verify_signature() {$/,/^}$/p' "$here/install.sh"
}

if [ -z "$(extract)" ]; then
	echo "test-install-verify: verify_signature not found in install.sh" >&2
	exit 1
fi

# run <name> <cosign-present> <sig-available> <cosign-verify-rc> <require>
# Echoes the function's output; returns its exit status.
run() {
	local cosign_present="$2" sig_available="$3" verify_rc="$4" require="$5"
	# SC2034/SC2329: every variable and function below is consumed by the
	# code `extract` pulls out of install.sh, which shellcheck cannot see
	# from here. That indirection is the point -- the test runs what ships.
	# shellcheck disable=SC2034,SC2329
	(
		set +e
		REPO="packetloss404/packetcode"
		VERSION="v9.9.9"
		CHECKSUMS_URL="https://example.invalid/checksums.txt"
		TMPDIR="$(mktemp -d)"
		trap 'rm -rf "$TMPDIR"' EXIT
		REQUIRE_SIGNATURE="$require"
		export REQUIRE_SIGNATURE

		# Stubs shadow the real commands. `command -v` finds shell functions,
		# which is what lets the "cosign is not installed" branch be reached.
		if [ "$cosign_present" = "yes" ]; then
			eval 'cosign() { return '"$verify_rc"'; }'
		fi
		curl() {
			local out=""
			# Mimic `curl -o <file>` closely enough that the function's own
			# redirection and error handling are what is under test.
			while [ $# -gt 0 ]; do
				case "$1" in
				-o)
					out="$2"
					shift 2
					;;
				*) shift ;;
				esac
			done
			[ "$sig_available" = "yes" ] || return 22
			[ -n "$out" ] && : > "$out"
			return 0
		}

		eval "$(extract)"
		verify_signature
	)
}

echo "install.sh signature verification"

# A good signature: quiet success, and it must say so.
out="$(run "verified" yes yes 0 "")"
rc=$?
check "a valid signature passes" "$rc" "0"
case "$out" in
*"signature verified"*) check "a valid signature is reported" "yes" "yes" ;;
*) check "a valid signature is reported" "$out" "signature verified..." ;;
esac

# The one that matters. A present-but-invalid signature must stop the install
# outright -- never be softened into the "unsigned release" note.
out="$(run "forged" yes yes 1 "")"
rc=$?
check "an invalid signature aborts" "$rc" "1"
case "$out$rc" in
*"no signature"*) check "an invalid signature is not reported as absent" "reported absent" "reported invalid" ;;
*) check "an invalid signature is not reported as absent" "reported invalid" "reported invalid" ;;
esac

# An old release genuinely has no signature. That is a note by default...
out="$(run "unsigned release" yes no 0 "")"
check "an unsigned release installs by default" "$?" "0"

# ...and a refusal when the caller asked for one.
run "unsigned release, required" yes no 0 "1" >/dev/null
check "REQUIRE_SIGNATURE=1 rejects an unsigned release" "$?" "1"

# No cosign: proceed, because the checksum check still ran and most machines
# have no cosign -- but only when the caller has not demanded a signature.
run "no cosign" no yes 0 "" >/dev/null
check "a missing cosign does not block installing" "$?" "0"

run "no cosign, required" no yes 0 "1" >/dev/null
check "REQUIRE_SIGNATURE=1 rejects a missing cosign" "$?" "1"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
