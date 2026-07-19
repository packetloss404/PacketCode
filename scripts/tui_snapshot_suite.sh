#!/bin/sh
set -eu

TARGET=${1:-packetcode}
CAPTURE=${CAPTURE:-python3 scripts/tui_capture.py}

# Safe, credential-free states. PACKETCODE_CMD/CLAUDE_CMD may point at wrappers
# that provide deterministic fake-provider responses for lifecycle states.
$CAPTURE --target "$TARGET" --scenario welcome
$CAPTURE --target "$TARGET" --scenario idle-focused --keys 'hello'
$CAPTURE --target "$TARGET" --scenario autocomplete --keys '/co'
$CAPTURE --target "$TARGET" --scenario narrow-layout --keys '/help'

# Rich lifecycle scenarios are enabled by an explicit deterministic wrapper.
# It must not contain credentials and should expose the named state on startup.
if [ -n "${TUI_FIXTURE_CMD:-}" ]; then
  for state in user-assistant thinking streaming tool-running tool-result approval error cancelled queued compacting compacted normal accept-edits auto plan bypass agents workflows; do
    $CAPTURE --target "$TARGET" --scenario "$state" --command "$TUI_FIXTURE_CMD --tui-fixture=$state"
  done
fi
