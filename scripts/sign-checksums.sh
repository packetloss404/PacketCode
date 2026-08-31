#!/usr/bin/env sh
# Sign checksums.txt with Sigstore, keyless.
#
#   sign-checksums.sh <artifact> <signature-out> <certificate-out>
#
# This is the half the installers were missing. They verify an archive against
# checksums.txt, which establishes that the download was not corrupted -- but
# whoever could serve a forged archive could serve a matching checksums.txt with
# it, and the check would pass. Signing the checksum file is what makes the
# chain terminate somewhere an attacker does not control.
#
# Keyless on purpose. cosign exchanges the workflow's GitHub OIDC token for a
# certificate that lives for minutes and records the exchange in a public
# transparency log. There is no signing key in a secret to leak, and no rotation
# to forget.
#
# Skipped, loudly, when there is no OIDC token: a fork, a local snapshot build,
# and a pull request all lack one, and none of them should fail for it.
# REQUIRE_SIGNING=1 turns the skip into an error for the real release.
set -eu

artifact="${1:?usage: sign-checksums.sh <artifact> <signature> <certificate>}"
signature="${2:?usage: sign-checksums.sh <artifact> <signature> <certificate>}"
certificate="${3:?usage: sign-checksums.sh <artifact> <signature> <certificate>}"

say() { printf 'sign-checksums: %s\n' "$1" >&2; }

# ACTIONS_ID_TOKEN_REQUEST_TOKEN is what `permissions: id-token: write` grants.
# Its absence is the precise, checkable statement of "this run cannot sign",
# which is better than inferring it from cosign failing later.
if [ -z "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ] && [ -z "${COSIGN_KEYLESS_FORCE:-}" ]; then
	if [ "${REQUIRE_SIGNING:-}" = "1" ]; then
		say "REQUIRE_SIGNING=1 but no OIDC token is available; refusing to publish unsigned checksums"
		exit 1
	fi
	say "no OIDC token; leaving $artifact unsigned"
	exit 0
fi

if ! command -v cosign >/dev/null 2>&1; then
	say "cosign is not installed, but this run was expected to sign"
	exit 1
fi

say "signing $artifact"
cosign sign-blob \
	--yes \
	--output-signature="$signature" \
	--output-certificate="$certificate" \
	"$artifact"

# Prove the signature verifies before the release publishes it. A signature
# nobody checked is indistinguishable from one that does not work, and the
# first person to find out would be someone following our own instructions.
if [ -n "${GITHUB_REPOSITORY:-}" ] && [ -n "${GITHUB_REF_NAME:-}" ]; then
	say "verifying the signature that was just produced"
	cosign verify-blob "$artifact" \
		--signature "$signature" \
		--certificate "$certificate" \
		--certificate-identity-regexp "https://github.com/${GITHUB_REPOSITORY}/\.github/workflows/.*" \
		--certificate-oidc-issuer https://token.actions.githubusercontent.com
fi

say "signed $artifact"
