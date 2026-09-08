#!/usr/bin/env sh
# Exercise both sides of the secret-presence branch with synthetic values.
set -eu
script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"

for macos in '' 'FAKE_MACOS_CERTIFICATE'; do
	for windows in '' 'FAKE_WINDOWS_CERTIFICATE'; do
		output="$(MACOS_SIGN_P12="$macos" WINDOWS_SIGN_P12="$windows" REQUIRE_SIGNING=1 \
			sh "$script_dir/report-signing-config.sh")"
		case "$output" in
			*FAKE_MACOS_CERTIFICATE*|*FAKE_WINDOWS_CERTIFICATE*)
				echo 'FAIL: certificate content appeared in signing summary' >&2
				exit 1 ;;
		esac
		if [ -n "$macos" ]; then
			printf '%s\n' "$output" | grep -Fq '| macOS binaries (Developer ID + notarization) | yes |'
		else
			printf '%s\n' "$output" | grep -Fq 'MACOS_SIGN_P12 is not set'
		fi
		if [ -n "$windows" ]; then
			printf '%s\n' "$output" | grep -Fq '| Windows binaries (Authenticode) | yes |'
		else
			printf '%s\n' "$output" | grep -Fq 'WINDOWS_SIGN_P12 is not set'
		fi
		printf '%s\n' "$output" | grep -Fq 'REQUIRE_SIGNING=1.'
	done
done

unset MACOS_SIGN_P12 WINDOWS_SIGN_P12 REQUIRE_SIGNING
output="$(sh "$script_dir/report-signing-config.sh")"
printf '%s\n' "$output" | grep -Fq 'MACOS_SIGN_P12 is not set'
printf '%s\n' "$output" | grep -Fq 'WINDOWS_SIGN_P12 is not set'
printf '%s\n' "$output" | grep -Fq 'REQUIRE_SIGNING=0.'
echo 'Signing configuration summary tests passed.'
