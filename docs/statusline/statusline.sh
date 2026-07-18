#!/bin/bash
# packetcode status line — molded from the Claude Code statusline in
# claude-code-tools/claudetools-bash/statusline/statusline.sh.
#
# Shows: [Provider·Model] Context% (tokens) | Dir | Git Branch | Cost | Op
#
# Wire it up in ~/.packetcode/config.toml:
#   [statusline]
#   command = "$HOME/projects/packetcode/docs/statusline/statusline.sh"
#   enabled = true
#   timeout_sec = 2
#
# packetcode also emits a Claude Code-compatible superset, so your existing
# ~/.claude/statusline/statusline.sh works verbatim — point `command` at it if
# you prefer that exact look. This molded copy adds packetcode-native segments
# (provider, cost, live operation) that Claude Code's snapshot does not carry.
#
# Keep this file LF-encoded; CRLF breaks bash on Unix.

input=$(cat)

MODEL=$(echo "$input" | jq -r '.model.display_name // .model.id // "Unknown"')
PROVIDER=$(echo "$input" | jq -r '.provider.display_name // .provider.slug // ""')

# packetcode pre-computes the context percentage; fall back to used/max.
PERCENT=$(echo "$input" | jq -r '.context_window.used_percentage // 0')
USED=$(echo "$input" | jq -r '.context_window.used // 0')
MAX=$(echo "$input" | jq -r '.context_window.max // 0')
if [ "$PERCENT" = "0" ] && [ "$MAX" -gt 0 ]; then
    PERCENT=$((USED * 100 / MAX))
fi
[ "$PERCENT" -lt 0 ] && PERCENT=0
[ "$PERCENT" -gt 100 ] && PERCENT=100

USED_K=$((USED / 1000))
MAX_K=$((MAX / 1000))
TOKEN_DISPLAY="${USED_K}K/${MAX_K}K"

# Color-coded context indicator.
if [ "$PERCENT" -ge 80 ]; then
    CONTEXT_ICON="🔴"
elif [ "$PERCENT" -ge 60 ]; then
    CONTEXT_ICON="🟡"
else
    CONTEXT_ICON="🟢"
fi

# Directory + git branch. packetcode passes working_dir; .cwd is the Claude
# Code alias. Prefer packetcode's git_branch, fall back to asking git.
CWD=$(echo "$input" | jq -r '.working_dir // .cwd // "."')
DIR_NAME=$(basename "$CWD")
GIT_BRANCH=$(echo "$input" | jq -r '.git_branch // ""')
if [ -z "$GIT_BRANCH" ]; then
    GIT_BRANCH=$(cd "$CWD" 2>/dev/null && git branch --show-current 2>/dev/null || echo "")
fi
if [ -n "$GIT_BRANCH" ]; then
    GIT_DISPLAY="🌿 $GIT_BRANCH"
else
    GIT_DISPLAY="🌿 -"
fi

# Model label, prefixed with the provider when one is present. The separator
# is held in its own variable: gluing a multibyte character directly onto a
# parameter expansion ("$PROVIDER·$MODEL") corrupts it under the non-UTF-8
# locale packetcode's `sh -c` may run with.
SEP="·"
if [ -n "$PROVIDER" ]; then
    MODEL_LABEL="$PROVIDER$SEP$MODEL"
else
    MODEL_LABEL="$MODEL"
fi

# Session cost — hidden at $0 (e.g. Codex subscription bills a flat rate).
COST=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
COST_DISPLAY=""
if awk "BEGIN{exit !($COST > 0.0005)}"; then
    COST_DISPLAY=$(printf "💲%.2f" "$COST")
fi

# Live operation indicator (thinking / running a tool), with elapsed seconds.
OP_ACTIVE=$(echo "$input" | jq -r '.operation.active // false')
OP_DISPLAY=""
if [ "$OP_ACTIVE" = "true" ]; then
    OP_LABEL=$(echo "$input" | jq -r '.operation.label // "working"')
    OP_ELAPSED=$(echo "$input" | jq -r '.operation.elapsed_seconds // 0')
    QUEUED=$(echo "$input" | jq -r '.operation.queued_inputs // 0')
    OP_DISPLAY="◷ ${OP_LABEL} ${OP_ELAPSED}s"
    [ "$QUEUED" -gt 0 ] && OP_DISPLAY="$OP_DISPLAY (+${QUEUED} queued)"
fi

# Assemble; optional segments appear only when populated.
LINE="[$MODEL_LABEL] $CONTEXT_ICON ${PERCENT}% ($TOKEN_DISPLAY) | 📂 $DIR_NAME | $GIT_DISPLAY"
[ -n "$COST_DISPLAY" ] && LINE="$LINE | $COST_DISPLAY"
[ -n "$OP_DISPLAY" ] && LINE="$LINE | $OP_DISPLAY"
echo "$LINE"
