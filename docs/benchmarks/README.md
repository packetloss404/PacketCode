# Benchmarks

Benchmarks in this directory record a reproducible question, environment,
method, and raw machine-readable evidence. They are diagnostic evidence, not a
product-speed promise: live-provider latency, provider routing, cache state,
and model choices all change.

## Headless `run` versus ACP

Build PacketCode once, then run the development harness so compilation is not
charged to either path:

```powershell
go build -o "$env:TEMP\packetcode-bench.exe" ./cmd/packetcode
go run ./tools/benchrun `
  -packetcode "$env:TEMP\packetcode-bench.exe" `
  -provider openai `
  -model gpt-5.6-sol `
  -permission-mode read-only `
  -runs 3 `
  -output docs/benchmarks/data/run-vs-acp.json
```

The harness sends the same bounded prompt through fresh sessions, alternates
which path runs first, and requires zero approvals. It copies `config.toml`
and an optional home-level `.env` into a temporary `PACKETCODE_HOME`, stores no
model response text or credential material in the report, and removes that
temporary home on exit.

The primary comparison is process start through completed response. `run`'s
reported elapsed time and ACP's initialize, session creation, and prompt
round-trip times are included as supporting measurements, but they do not have
identical boundaries and must not be subtracted from one another as if they
did. Usage and provider/tool-call counts are read from the persisted session
record before the isolated home is removed.

Recorded runs:

- [2026-09-01: `run` versus ACP on `gpt-5.6-sol`](run-vs-acp-2026-09-01.md)
