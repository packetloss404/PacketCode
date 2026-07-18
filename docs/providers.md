# Providers And Models

packetcode supports these provider slugs:

| Slug | Needs key | Notes |
| --- | --- | --- |
| `openai` | Yes | Uses the OpenAI API. |
| `codex` | No | Uses an OpenAI Codex ChatGPT subscription via `~/.codex/auth.json`. See [Codex subscription](#codex-subscription). |
| `anthropic` | Yes | Uses the Anthropic Claude Messages API. |
| `gemini` | Yes | Uses the Google Gemini API. |
| `minimax` | Yes | Uses MiniMax's OpenAI-compatible API surface. |
| `openrouter` | Yes | Lists models and pricing from OpenRouter. |
| `ollama` | No | Uses a reachable Ollama server. |
| custom slug | Optional | Uses a user-configured OpenAI-compatible `/v1` endpoint. |

## Codex Subscription

The `codex` provider lets you drive packetcode with an OpenAI Codex **ChatGPT
subscription** (Plus/Pro/Team/Enterprise) instead of a per-token API key. It
does not implement its own login — it reuses the OAuth credentials the official
[Codex CLI](https://github.com/openai/codex) writes when you sign in.

**Setup**

1. Install the Codex CLI and run `codex login`, choosing **Sign in with
   ChatGPT** (not the API-key option). This writes OAuth tokens to
   `~/.codex/auth.json`.
2. In packetcode, switch with `/provider codex`, or launch with
   `packetcode --provider codex`.

That's it — there is no key to paste. The provider row shows `ChatGPT login`
in the picker, and `packetcode doctor` reports the credential source as the
`auth.json` path.

**How it works**

- packetcode reads the access token, refresh token, and account id from
  `~/.codex/auth.json` on every request, so it stays in sync with the Codex CLI.
- When the access token expires, packetcode exchanges the refresh token for a
  new one against the OpenAI OAuth endpoint and writes the result back to
  `auth.json` (preserving the file's other fields).
- Requests go to the ChatGPT backend's **Responses API**, not the standard
  `/chat/completions` endpoint.
- The model list is read from the Codex CLI's `models_cache.json` (next to
  `auth.json`), so packetcode's model picker stays in sync with whatever your
  account can use — e.g. `gpt-5.6-sol` (default), `gpt-5.6-terra`, `gpt-5.5`.
  Each model is sent its Codex-default reasoning effort. If the cache is
  missing, a built-in fallback list is used.
- Cost is reported as `$0` because a subscription bills a flat rate rather than
  per token. Your ChatGPT plan's usage limits still apply.

**Notes and limits**

- Requires the Codex CLI login; if `auth.json` is missing or holds only an
  `OPENAI_API_KEY` (API-key mode), packetcode explains what to do. For per-token
  API billing, use the `openai` provider instead.
- Non-standard `CODEX_HOME` layouts are honored (the same env var the Codex CLI
  uses). You can also point at a specific file with `host` under
  `[providers.codex]` in `~/.packetcode/config.toml`.

## Configure Keys

First run configures one provider. To add or update another provider later:

1. Open the provider picker with `Ctrl+P` or `/provider`.
2. Focus the provider row.
3. Press `Ctrl+A`.
4. Paste the API key.

The key is validated, saved to `~/.packetcode/config.toml`, and the picker reopens with the row marked `key present`.

`/provider add` opens that same picker. `/provider add <slug>` opens the same key prompt directly for a provider slug.

You can also set keys with environment variables:

```text
PACKETCODE_OPENAI_API_KEY
PACKETCODE_ANTHROPIC_API_KEY
PACKETCODE_GEMINI_API_KEY
PACKETCODE_MINIMAX_API_KEY
PACKETCODE_OPENROUTER_API_KEY
PACKETCODE_MY_PROVIDER_API_KEY
```

Environment variables win over config file keys. Custom provider slugs are
normalized to `PACKETCODE_<SLUG>_API_KEY`, with non-alphanumeric characters
converted to `_`; set `api_key_env` to use a different variable.

## Switch Providers

| Action | Command |
| --- | --- |
| Open provider picker | `Ctrl+P` or `/provider` |
| Add/update provider key | `Ctrl+P` then `Ctrl+A`, `/provider add`, or `/provider add <slug>` |
| Switch directly | `/provider <slug>` |
| Open model picker | `Ctrl+M` or `/model` |
| Switch model directly | `/model <id>` |

When switching providers, packetcode uses that provider's saved `default_model`. If no default model is saved, it falls back to the first model returned by the provider's model list. The chosen provider/model is persisted as the new default.

## Config Example

```toml
[default]
provider = "openai"
model = "gpt-5.6-sol"

[providers.openai]
api_key = "sk-..."
default_model = "gpt-5.6-sol"

[providers.anthropic]
api_key = "sk-ant-..."
default_model = "claude-opus-4-8"

[providers.gemini]
api_key = "AI..."
default_model = "gemini-2.5-pro"

[providers.minimax]
api_key = "sk-..."
default_model = "MiniMax-M3"

[providers.ollama]
host = "http://localhost:11434"
default_model = "qwen2.5-coder:14b"
# Optional tuning — all default to packetcode's smart values, so a stock
# local install needs none of these:
# num_ctx = 65536        # fixed context window; omit to auto-size per request (capped to the model's max)
# keep_alive = "30m"     # how long the model stays loaded; "-1" pins it, "0" unloads immediately
# temperature = 0.2      # omit to use the model's own default
```

A stock local Ollama needs zero configuration — packetcode auto-sizes the context window to each prompt (capped to the model's real maximum, read from `/api/show`), keeps the model loaded for 30 minutes to avoid reload latency, and detects per-model tool support automatically. The tuning keys above are only for overriding those defaults.

`host` is only used for Ollama. If omitted, packetcode defaults to `http://localhost:11434`. A bare host like `ollama.internal` is normalized to `http://ollama.internal:11434`. You can also set `PACKETCODE_OLLAMA_HOST` to override the saved host for one machine.

## Custom OpenAI-Compatible Providers

Any service that implements OpenAI-compatible `/models` and
`/chat/completions` endpoints can be added as a provider:

```toml
[providers.localai]
type = "openai_compatible"
display_name = "LocalAI"
base_url = "http://localhost:8080/v1"
default_model = "coder-large"
api_key_required = false

[[providers.localai.models]]
id = "coder-large"
context_window = 32768
supports_tools = true
```

For hosted gateways, keep `api_key_required` omitted or set it to `true` and
store the key in config or an env var:

```toml
[providers.acme]
type = "openai_compatible"
display_name = "Acme Gateway"
base_url = "https://llm.acme.example/v1"
api_key_env = "ACME_LLM_TOKEN"
default_model = "acme-coder"
headers = { "X-Workspace" = "packetcode" }
```

Static `models` entries are used as a fallback when `/models` is unavailable
or incomplete. Unknown custom model prices default to zero and context defaults
to 128k tokens unless configured.
