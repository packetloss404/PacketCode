#!/usr/bin/env bash
# packetcode end-to-end smoke test.
#
# What this is for. The unit suite proves each package in isolation; this
# proves the seams: that a configured credential actually reaches the wire,
# that an approved write lands on disk, that an unapproved one does not, and
# that the permission floors and the secret-file refusal hold when the whole
# loop runs. Those are the things that break silently after a merge and that no
# single package test can see.
#
# It needs no credentials and no network. tools/smokestub is a stdlib-only
# OpenAI-compatible server that answers packetcode's real provider client, so
# the agent loop under test is the production one. No module dependency is
# added by any of this: bash, the Go toolchain, and this repository's own code
# are the whole requirement.
#
# The audit brief asked for "login, one authenticated write, one failure path,
# and a webhook rejection". packetcode has no login and no webhooks, so those
# map onto what exists and the substitutions are stated in the output:
#
#   login              -> provider credential resolved from the environment and
#                         accepted by the server (and rejected when wrong)
#   authenticated write-> an approved write_file that lands on disk
#   failure path       -> the same write refused, failing closed with exit 3
#   webhook rejection  -> no webhooks exist; the nearest untrusted-input
#                         refusals are asserted instead: the dotenv secret
#                         refusal and the compound-command deny floor
#
# Usage:  ./smoke.sh          (from anywhere; it locates the repo itself)
# Exit:   0 all checks passed, 1 otherwise.

set -uo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$PWD"

pass=0
fail=0

check() { # check <name> <got> <want>
	if [ "$2" = "$3" ]; then
		printf '  ok   %s\n' "$1"
		pass=$((pass + 1))
	else
		printf '  FAIL %s\n         got:  %s\n         want: %s\n' "$1" "$2" "$3"
		fail=$((fail + 1))
	fi
}

contains() { # contains <name> <haystack> <needle>
	case "$2" in
	*"$3"*) check "$1" "present" "present" ;;
	*) check "$1" "absent" "present" ;;
	esac
}

absent() { # absent <name> <haystack> <needle>
	case "$2" in
	*"$3"*) check "$1" "present" "absent" ;;
	*) check "$1" "absent" "absent" ;;
	esac
}

EXE=""
case "$(uname -s 2>/dev/null || echo unknown)" in
MINGW* | MSYS* | CYGWIN*) EXE=".exe" ;;
esac

native() { # native <posix-path> -> a path the Go binary will accept
	if command -v cygpath >/dev/null 2>&1; then cygpath -w "$1"; else printf '%s' "$1"; fi
}

WORK="$(mktemp -d 2>/dev/null || mktemp -d -t packetcode-smoke)"
STUB_PID=""
cleanup() {
	[ -n "$STUB_PID" ] && kill "$STUB_PID" 2>/dev/null
	rm -rf "$WORK"
}
trap cleanup EXIT

BIN="$WORK/packetcode$EXE"
STUB="$WORK/smokestub$EXE"
HOME_DIR="$WORK/home"
PROJECT="$WORK/project"
mkdir -p "$HOME_DIR" "$PROJECT"

echo "packetcode smoke test"
echo "  repo:  $REPO"
echo "  work:  $WORK"
echo

echo "building"
if ! go build -o "$BIN" ./cmd/packetcode; then
	echo "  FAIL build packetcode" && exit 1
fi
if ! go build -o "$STUB" ./tools/smokestub; then
	echo "  FAIL build smokestub" && exit 1
fi
echo "  ok   built packetcode and smokestub"
echo

TOKEN="smoke-token-$$"
ADDR_FILE="$WORK/addr"
"$STUB" -token "$TOKEN" -addr-file "$ADDR_FILE" >"$WORK/stub.log" 2>&1 &
STUB_PID=$!
waited=0
while [ ! -s "$ADDR_FILE" ] && [ "$waited" -lt 100 ]; do
	sleep 0.1
	waited=$((waited + 1))
done
if [ ! -s "$ADDR_FILE" ]; then
	echo "  FAIL smokestub did not start; log follows" && cat "$WORK/stub.log" && exit 1
fi
BASE="$(cat "$ADDR_FILE")"

# An isolated home and an isolated project, so a run can never touch the
# operator's real sessions, jobs, config, or checkout.
export PACKETCODE_HOME="$(native "$HOME_DIR")"
export PACKETCODE_SMOKE_TOKEN="$TOKEN"

write_config() { # write_config <extra toml>
	cat >"$HOME_DIR/config.toml" <<TOML
[default]
provider = "smokestub"
model = "smoke-model"

[providers.smokestub]
type = "openai_compatible"
display_name = "Smoke Stub"
base_url = "$BASE/v1"
api_key_env = "PACKETCODE_SMOKE_TOKEN"
default_model = "smoke-model"

[[providers.smokestub.models]]
id = "smoke-model"
context_window = 128000
supports_tools = true
${1:-}
TOML
}
write_config

# git init pins the project root: packetcode resolves its root to the enclosing
# repository, and without this a temp directory inside someone's checkout would
# make the write test target that checkout instead.
(cd "$PROJECT" && git init -q 2>/dev/null)
printf 'SMOKE_SECRET_VALUE=do-not-leak-me\n' >"$PROJECT/.env"

RC=0
OUT=""
run_pc() { # run_pc <permission-mode> <prompt>
	OUT="$(cd "$PROJECT" && "$BIN" run --permission-mode "$1" "$2" 2>&1)"
	RC=$?
}

echo "1. binary and command surface"
OUT="$("$BIN" --version 2>&1)"
RC=$?
check "--version exits 0" "$RC" "0"
contains "--version names the program" "$OUT" "packetcode"
OUT="$("$BIN" --help 2>&1)"
for cmd in run doctor skills acp sugar; do
	contains "--help lists $cmd" "$OUT" "$cmd"
done
echo

echo "2. configuration and diagnostics"
OUT="$("$BIN" doctor --check config --json 2>&1)"
RC=$?
check "doctor --check config exits 0 with a valid config" "$RC" "0"
contains "doctor reports the boot validation check" "$OUT" "config.validation"
contains "doctor emits schema-versioned JSON" "$OUT" '"schema_version"'
echo

echo "3. credential resolution (this codebase has no login)"
run_pc auto "SMOKE_PLAIN please reply"
check "a run with the configured credential exits 0" "$RC" "0"
contains "the model reply reaches stdout" "$OUT" "SMOKE_PLAIN_REPLY"
absent "the credential is never printed" "$OUT" "$TOKEN"

# Explicitly saved and restored: bash does not agree with itself about whether
# an assignment prefixing a function call persists after it returns.
GOOD_TOKEN="$PACKETCODE_SMOKE_TOKEN"
export PACKETCODE_SMOKE_TOKEN="wrong-token"
run_pc auto "SMOKE_PLAIN please reply"
export PACKETCODE_SMOKE_TOKEN="$GOOD_TOKEN"
if [ "$RC" -ne 0 ]; then check "a wrong credential fails the run" "nonzero" "nonzero"; else check "a wrong credential fails the run" "0" "nonzero"; fi
contains "the rejection names the status" "$OUT" "401"
echo

echo "4. authenticated write"
rm -f "$PROJECT/smoke-artifact.txt"
run_pc accept-edits "SMOKE_WRITE create the artifact"
check "an approved write run exits 0" "$RC" "0"
if [ -f "$PROJECT/smoke-artifact.txt" ]; then check "the file was written" "written" "written"; else check "the file was written" "missing" "written"; fi
contains "the file holds what the tool was given" "$(cat "$PROJECT/smoke-artifact.txt" 2>/dev/null)" "written-by-smoke"
echo

echo "5. failure paths (approval unavailable, and read-only)"
rm -f "$PROJECT/smoke-artifact.txt"
run_pc ask "SMOKE_WRITE create the artifact"
check "an unapproved write exits 3, not 0" "$RC" "3"
if [ -f "$PROJECT/smoke-artifact.txt" ]; then check "no file is written when approval is unavailable" "written" "absent"; else check "no file is written when approval is unavailable" "absent" "absent"; fi

rm -f "$PROJECT/smoke-artifact.txt"
run_pc read-only "SMOKE_WRITE create the artifact"
if [ -f "$PROJECT/smoke-artifact.txt" ]; then check "read-only denies the write" "written" "absent"; else check "read-only denies the write" "absent" "absent"; fi
contains "the model is told it was denied" "$OUT" "permission denied"
echo

echo "6. untrusted-input refusals (this codebase has no webhooks to reject)"
run_pc auto "SMOKE_READ_ENV read the dotenv file"
check "the dotenv read run completes" "$RC" "0"
contains "read_file refuses the secret file" "$OUT" "refusing to read"
absent "the secret never reaches the transcript" "$OUT" "do-not-leak-me"

write_config '
[[permissions.rules]]
tool = "execute_command"
action = "deny"
command_prefix = ["echo", "SMOKE_DENIED_MARKER"]
reason = "smoke test deny floor"'
run_pc auto "SMOKE_DENY_SHELL run the denied command"
absent "a compound command cannot slip past the deny floor" "$OUT" "SMOKE_DENIED_MARKER"
contains "the denial is reported to the model" "$OUT" "permission denied"
write_config
echo

printf '%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
