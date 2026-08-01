#!/bin/sh
set -eu

MODE=${1:-check}
case "$MODE" in
  check|update) ;;
  *) echo "usage: $0 [check|update]" >&2; exit 2 ;;
esac

CAPTURE=${CAPTURE:-python3 scripts/tui_capture.py}
FIXTURE_CMD=${TUI_FIXTURE_CMD:-./bin/packetcode}
GOLDEN_ROOT=${TUI_GOLDEN_ROOT:-testdata/tui/golden}
if [ "$MODE" = update ] && [ "$GOLDEN_ROOT" != testdata/tui/golden ]; then
  echo "update mode only writes testdata/tui/golden" >&2
  exit 1
fi
TMP_ROOT=$(mktemp -d)
trap 'rm -rf "$TMP_ROOT"' EXIT HUP INT TERM

# These states cover the shared chrome and the tallest/highest-risk overlays
# without committing local config, account data, provider output, or raw ANSI.
capture_state() {
  state=$1
  shift
  $CAPTURE \
    --target packetcode \
    --scenario "$state" \
    --command "$FIXTURE_CMD --tui-fixture=$state" \
    --output "$TMP_ROOT" \
    --size 72x24 \
    --size 100x30 \
    "$@" \
    --protocol-check
}

for state in user-assistant streaming tool-result approval queued plan agents workflows; do
  if [ "$state" = user-assistant ]; then
    capture_state "$state" --expect-text "café · 中文 · 👩🏽‍💻"
  else
    capture_state "$state"
  fi
done

# One live SIGWINCH path proves components reflow after startup rather than
# merely receiving the requested initial size.
$CAPTURE \
  --target packetcode \
  --scenario resize \
  --command "$FIXTURE_CMD --tui-fixture=user-assistant" \
  --output "$TMP_ROOT" \
  --size 100x30 \
  --resize 72x24 \
  --expect-text "café · 中文 · 👩🏽‍💻" \
  --protocol-check

# A shrinking inline terminal may commit pre-resize rows to scrollback. The
# normalized post-SIGWINCH live frame must contain exactly one current status
# row, and its stable input/status tail must match a clean target-width start.
direct="$TMP_ROOT/packetcode/user-assistant/72x24.txt"
resized="$TMP_ROOT/packetcode/resize/100x30-to-72x24.txt"
[ "$(grep -F -c '[Codex (ChatGPT)·gpt-5.6-sol]' "$resized")" -eq 1 ] || {
  echo "resize capture contains stale or missing status chrome" >&2
  exit 1
}
direct_tail="$TMP_ROOT/direct-tail.txt"
resized_tail="$TMP_ROOT/resized-tail.txt"
awk '/^─/{copy=1} /^-- cell styles --/{exit} copy' "$direct" > "$direct_tail"
awk '/^─/{copy=1} /^-- cell styles --/{exit} copy' "$resized" > "$resized_tail"
cmp "$direct_tail" "$resized_tail"
rm -f "$direct_tail" "$resized_tail"

# Raw streams are safety-checked above but intentionally never promoted.
find "$TMP_ROOT" -type f -name '*.ansi' -delete

if [ "$MODE" = update ]; then
  mkdir -p "$GOLDEN_ROOT"
  managed="$GOLDEN_ROOT/packetcode"
  parent=$(dirname "$managed")
  parent_abs=$(cd "$parent" && pwd -P)
  workspace_abs=$(pwd -P)
  expected_parent="$workspace_abs/testdata/tui/golden"
  if [ "$(basename "$managed")" != packetcode ] ||
     [ "$parent_abs" != "$expected_parent" ]; then
    echo "refusing to replace unexpected golden path: $managed" >&2
    exit 1
  fi
  managed_abs="$parent_abs/packetcode"
  rm -rf -- "$managed_abs"
  cp -R "$TMP_ROOT/packetcode" "$managed_abs"
  echo "updated reviewed TUI goldens in $GOLDEN_ROOT"
  exit 0
fi

if [ ! -d "$GOLDEN_ROOT" ]; then
  echo "missing TUI golden directory: $GOLDEN_ROOT" >&2
  exit 1
fi
diff -ru "$GOLDEN_ROOT" "$TMP_ROOT"
echo "TUI goldens match"
