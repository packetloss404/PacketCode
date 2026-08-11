# Cache affinity

Packetcode keeps model-facing history byte-stable across turns and resumes.
Full tool output remains in the local session and UI. When a tool result is
large, Packetcode creates a bounded `model_content` projection as the result is
first persisted. That projection never changes merely because a newer tool ran.

The default model projection limit is 65,536 bytes. Set
`PACKETCODE_MODEL_TOOL_RESULT_LIMIT_BYTES` before starting Packetcode to choose
a value from 16,384 through 1,048,576 bytes. The setting applies only when a
projection is first created; existing persisted projections are not rewritten.
This preserves cache lineage across configuration changes and session resume.

Sugar requests carry a private validated `sugar_cache` envelope containing the
stable session ID, canonical system/tool prefix fingerprint, leading stable
system-message boundary, and compaction generation. Direct providers never
receive this metadata. Explicit compaction keeps the session ID and atomically
increments the generation.

Cache behavior is configured under `[sugar]` with `cache_mode` (`auto` or
`off`), `cache_retention` (`provider_default`, `5m`, `30m`, or `1h`), and
`privacy` (`standard` or `zdr_required`). The environment overrides are
`PACKETCODE_SUGAR_CACHE_MODE`, `PACKETCODE_SUGAR_CACHE_RETENTION`, and
`PACKETCODE_SUGAR_PRIVACY`. Environment wins over TOML, which wins over the
defaults above. Sugar workspace policy is authoritative and may tighten these
requests.

The Conduit runtime client matches Sugar's shadow-only run/event/continue API,
and is disabled by default. Set `[conduit].shadow_enabled = true` or
`PACKETCODE_CONDUIT_SHADOW=true` to opt in. Eligible `sugar/conduit` turns start
one shadow run, emit ordered coarse tool/validation/provider outcomes, and call
Continue for recommendation telemetry. A recommendation is never applied to
the live model choice. Endpoint failures disable the current shadow lifecycle
without interrupting chat.

Runtime event DTOs cannot carry prompts, code, file names, tool arguments,
command output, or specialist capsules. The bounded v1 specialist capsule is a
separate local-session artifact for a future explicit local model handoff. It
prioritizes task constraints, normalized relative paths, API/schema change
notes, exact failed-gate fingerprints, bounded redacted excerpts, unresolved
decisions, and evidence; it never replays the transcript or enters Sugar
telemetry.
