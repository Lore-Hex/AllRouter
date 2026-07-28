# AllRouter Setup

This is the linear reference. For an interactive agent flow, use `skills/allrouter-setup/SKILL.md`.

## 1. Install

```bash
brew tap Lore-Hex/homebrew-tap
brew install allrouter
```

Or download the latest release binary:

```text
https://github.com/Lore-Hex/AllRouter/releases/latest
```

Other install paths:

```bash
go install github.com/Lore-Hex/AllRouter/cmd/allrouter@latest
docker build -t allrouter:local .
```

## 2. Run

Local-only with Ollama, LM Studio, llama.cpp, or vLLM on a common localhost port:

```bash
allrouter
```

Local plus TrustedRouter burst:

```bash
export TRUSTEDROUTER_API_KEY="tr_..."
allrouter -tr-api-key "$TRUSTEDROUTER_API_KEY"
```

Explicit local URL:

```bash
allrouter -local-url http://127.0.0.1:11434
```

Local plus aliases for tool-facing cloud model names:

```bash
export TRUSTEDROUTER_API_KEY="tr_..."
allrouter -local-url http://127.0.0.1:11434 \
  -tr-api-key "$TRUSTEDROUTER_API_KEY" \
  -alias gpt-4o=llama3.2 \
  -alias anthropic/claude-haiku-4.5=qwen2.5-coder:32b \
  -savings-reference gpt-4o
```

The savings meter is an honest counterfactual: local tokens are priced only from TrustedRouter catalog prices, using the alias key first, then the requested TrustedRouter-known model, then `-savings-reference`. Without one of those price anchors, AllRouter counts tokens only and reports no saved dollars.

TrustedRouter-only:

```bash
export TRUSTEDROUTER_API_KEY="tr_..."
allrouter -no-autodetect -tr-api-key "$TRUSTEDROUTER_API_KEY"
```

Claude Code with BackupRouter mode:

```bash
export TRUSTEDROUTER_API_KEY="tr_..."
allrouter -preset backuprouter -no-autodetect
```

In the shell where you run Claude Code:

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8383"
export ANTHROPIC_AUTH_TOKEN="${ALLROUTER_TOKEN:-allrouter-local}"
export ANTHROPIC_MODEL="anthropic/claude-sonnet-5"
export ANTHROPIC_SMALL_FAST_MODEL="anthropic/claude-haiku-4.5"
claude
```

BackupRouter keeps the requested Claude model first, then tries
`moonshotai/kimi-k3` and `z-ai/glm-5.2`. Override that order with repeatable
`-backup-model` flags or `ALLROUTER_BACKUP_MODELS`. An explicit request `models`
array always wins.

Open `http://127.0.0.1:8383/ui` to configure the primary model,
Claude Code small task model, and ordered recovery chain. Profiles provide coherent
Balanced, Fast, Economy, Zero retention, and End to end encrypted starting
points. The searchable live TrustedRouter catalog shows price, context,
privacy, open-weight status, managed routes, and provider coverage; non-chat
and internal models are excluded.

Saving applies the recovery chain immediately and writes it to
`$XDG_CONFIG_HOME/allrouter/config.json` or
`~/.config/allrouter/config.json`. The primary and small task choices are
client-side roles, so the panel produces setup for Claude Code, Codex CLI, or
the ChatGPT Desktop Codex workspace. Claude Code receives both
`ANTHROPIC_MODEL` and `ANTHROPIC_SMALL_FAST_MODEL`; Codex receives the primary
model through a custom Responses API provider. CLI flags and
`ALLROUTER_BACKUP_MODELS` take precedence over the saved UI route after a restart.
Set `ALLROUTER_TOKEN` before launch to protect both the API and UI.

If the proxy is reachable from the internet, require a bearer token:

```bash
export ALLROUTER_TOKEN="$(openssl rand -hex 24)"
allrouter -local-url http://127.0.0.1:11434 -tr-api-key "$TRUSTEDROUTER_API_KEY" -token "$ALLROUTER_TOKEN"
```

Clients may authenticate with either `Authorization: Bearer $ALLROUTER_TOKEN` or `x-api-key: $ALLROUTER_TOKEN`.

## 2a. Docker Compose With Ollama

From the repository root:

```yaml
services:
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - ollama:/root/.ollama

  allrouter:
    build: .
    depends_on:
      - ollama
    environment:
      ALLROUTER_LOCAL_URL: http://ollama:11434
      TRUSTEDROUTER_API_KEY: ${TRUSTEDROUTER_API_KEY:-}
    ports:
      - "8383:8383"

volumes:
  ollama:
```

## 3. Verify

Run the operator smoke against your local Ollama install:

```bash
scripts/smoke.sh
```

By default it uses `ALLROUTER_LOCAL_URL=http://127.0.0.1:11434` and starts AllRouter on `127.0.0.1:8383`. Set `ALLROUTER_MODEL` if you want to force a specific local model.

Without `ALLROUTER_TOKEN`:

```bash
export ALLROUTER_HOST="http://127.0.0.1:8383"
curl -fsS "$ALLROUTER_HOST/healthz"
curl -fsS "$ALLROUTER_HOST/ui" >/dev/null
curl -fsS "$ALLROUTER_HOST/v1/models"
curl -is "$ALLROUTER_HOST/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"ping"}]}' \
  | awk 'BEGIN{found=0} /^X-AllRouter-Route:/ {print; found=1} END{exit found?0:1}'
```

With `ALLROUTER_TOKEN`:

```bash
export ALLROUTER_HOST="http://127.0.0.1:8383"
curl -fsS "$ALLROUTER_HOST/healthz"
curl -fsS -H "Authorization: Bearer $ALLROUTER_TOKEN" "$ALLROUTER_HOST/ui" >/dev/null
curl -fsS -H "Authorization: Bearer $ALLROUTER_TOKEN" "$ALLROUTER_HOST/v1/models"
curl -is "$ALLROUTER_HOST/v1/chat/completions" \
  -H "Authorization: Bearer $ALLROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"ping"}]}' \
  | awk 'BEGIN{found=0} /^X-AllRouter-Route:/ {print; found=1} END{exit found?0:1}'
```

Open `$ALLROUTER_HOST/ui` as the "it's working" screen. It is read-only and shows the savings odometer, local/cloud split, local capacity, cloud spend, and recent routing decisions.

With `x-api-key`:

```bash
curl -is "$ALLROUTER_HOST/v1/chat/completions" \
  -H "x-api-key: $ALLROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"ping"}]}' \
  | awk 'BEGIN{found=0} /^X-AllRouter-Route:/ {print; found=1} END{exit found?0:1}'
```

## 3a. Burst To Another OpenAI-Compatible Cloud

TrustedRouter is the default burst target, but `-tr-base-url` may point at any bearer-keyed OpenAI-compatible `/v1` base URL.

```bash
export TRUSTEDROUTER_API_KEY="<upstream bearer token>"
allrouter -local-url http://127.0.0.1:11434 \
  -tr-api-key "$TRUSTEDROUTER_API_KEY" \
  -tr-base-url "https://openrouter.ai/api/v1"
```

Savings/pricing features use the TrustedRouter catalog. If the configured burst upstream lacks `/v1/messages` or `/v1/responses`, AllRouter returns a clean `501 endpoint_not_supported` envelope for cloud passthrough requests. Aliased local `/v1/messages` requests do not require the burst upstream to support Anthropic Messages.

## 3b. Savings And Cloud Controls

Savings state is written to `$XDG_STATE_HOME/allrouter/state.json` or `~/.allrouter/state.json`; set `-state-file ""` to disable persistence.

BackupRouter UI configuration is written separately to
`$XDG_CONFIG_HOME/allrouter/config.json` or
`~/.config/allrouter/config.json`; set `-config-file ""` to disable UI
persistence.

Cloud egress modes:

```bash
# Normal local-first bursting.
allrouter -local-url http://127.0.0.1:11434 -tr-api-key "$TRUSTEDROUTER_API_KEY" -cloud auto

# No automatic bursts; only explicit non-local provider requests can use cloud.
allrouter -local-url http://127.0.0.1:11434 -tr-api-key "$TRUSTEDROUTER_API_KEY" -cloud explicit

# Disable cloud entirely.
allrouter -local-url http://127.0.0.1:11434 -tr-api-key "$TRUSTEDROUTER_API_KEY" -cloud off
```

Set a per-UTC-day cap:

```bash
allrouter -local-url http://127.0.0.1:11434 \
  -tr-api-key "$TRUSTEDROUTER_API_KEY" \
  -max-cloud-spend 1.00
```

Once priced cloud spend reaches the cap, cloud sends return `429 cloud_budget_exhausted` with `Retry-After` set to seconds until UTC midnight. Unpriced cloud usage counts `$0` toward this cap, and remains visible as unpriced token usage in `/stats`.

## 4. Expose With Ngrok

Use this only when the agent harness runs remotely and cannot reach localhost.

```bash
ngrok config add-authtoken "$NGROK_AUTHTOKEN"
ngrok http 8383
```

For a stable domain:

```bash
ngrok http --domain=<your-domain>.ngrok.app 8383
```

When internet-exposed, set `ALLROUTER_TOKEN`. Do not expose AllRouter without it.

```bash
export ALLROUTER_HOST="https://<your-domain>.ngrok.app"
curl -fsS -H "Authorization: Bearer $ALLROUTER_TOKEN" "$ALLROUTER_HOST/v1/models"
```

## 5. Wire A Harness

Use the public host for remote harnesses, for example `https://<your-domain>.ngrok.app`. Use `http://127.0.0.1:8383` for local harnesses.

When using a TrustedRouter SDK for Python, JavaScript, Swift, or Go against AllRouter, set both the inference base and the control base/catalog base to the AllRouter URL. If only inference is pointed at AllRouter, SDK catalog/account calls still go directly to the TrustedRouter control plane and bypass the proxy.

### Cursor

Settings -> Models -> OpenAI API override:

```text
Base URL: https://<host>/v1
API key: $ALLROUTER_TOKEN, or any string when ALLROUTER_TOKEN is unset
Models: local/llama3.2, anthropic/claude-haiku-4.5
```

With aliases, list the cloud-facing alias ids instead:

```text
Models: gpt-4o, anthropic/claude-haiku-4.5
```

Verify with a chat request:

```bash
curl -is "https://<host>/v1/chat/completions" \
  -H "Authorization: Bearer $ALLROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"ping"}]}' \
  | awk 'BEGIN{found=0} /^X-AllRouter-Route:/ {print; found=1} END{exit found?0:1}'
```

### Claude Code / Anthropic SDKs

For cloud-first Claude Code with automatic model fallback, use
[BackupRouter mode](#2-run). AllRouter can also run Claude Code against a
local OpenAI-compatible model by translating Anthropic `/v1/messages` to local
`/v1/chat/completions`. Add an alias from the Claude model id your client sends
to the local model name:

```bash
export TRUSTEDROUTER_API_KEY="tr_..."
allrouter -local-url http://127.0.0.1:11434 \
  -tr-api-key "$TRUSTEDROUTER_API_KEY" \
  -alias anthropic/claude-haiku-4.5=qwen2.5-coder:32b
```

If Claude Code sends a different id, use that exact id on the left side of `-alias`. When local is full or failing and cloud egress is allowed, AllRouter bursts the original Anthropic body to TrustedRouter.

```bash
export ANTHROPIC_BASE_URL="https://<host>"
export ANTHROPIC_AUTH_TOKEN="${ALLROUTER_TOKEN:-allrouter-local}"
export ANTHROPIC_MODEL="anthropic/claude-haiku-4.5"
```

Anthropic-family clients send `x-api-key`; AllRouter accepts it when `ALLROUTER_TOKEN` is set.

Verify:

```bash
curl -is "https://<host>/v1/messages" \
  -H "x-api-key: $ALLROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"anthropic/claude-haiku-4.5","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}' \
  | awk 'BEGIN{found=0} /^X-AllRouter-Route:/ {print; found=1} END{exit found?0:1}'
```

### Aider, OpenAI SDKs, OpenHands

```bash
export OPENAI_BASE_URL="https://<host>/v1"
export OPENAI_API_KEY="${ALLROUTER_TOKEN:-any-string}"
```

If your tool uses the older variable:

```bash
export OPENAI_API_BASE="https://<host>/v1"
```

Verify:

```bash
curl -is "https://<host>/v1/chat/completions" \
  -H "Authorization: Bearer $ALLROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"ping"}]}' \
  | awk 'BEGIN{found=0} /^X-AllRouter-Route:/ {print; found=1} END{exit found?0:1}'
```

### Codex CLI and ChatGPT Desktop

Codex CLI uses the Responses API and should be configured through the shared
user-level Codex configuration instead of only `OPENAI_BASE_URL`:

```toml
model = "trustedrouter/auto"
model_provider = "allrouter"

[model_providers.allrouter]
name = "AllRouter"
base_url = "http://127.0.0.1:8383/v1"
env_key = "ALLROUTER_TOKEN"
wire_api = "responses"
```

Save that block in `~/.codex/config.toml`, then launch:

```bash
export ALLROUTER_TOKEN="${ALLROUTER_TOKEN:-allrouter-local}"
codex
```

The ChatGPT Desktop Codex workspace uses the same configuration layers. In the
app, open **Settings > Configuration > Open config.toml**, add the provider,
make `ALLROUTER_TOKEN` available to the app, and restart it:

```bash
launchctl setenv ALLROUTER_TOKEN "${ALLROUTER_TOKEN:-allrouter-local}"
```

Ordinary ChatGPT
conversations cannot replace OpenAI's underlying model provider with
AllRouter; this setup applies specifically to the Codex workspace.

### Generic OpenAI-Compatible Integration

```text
Base URL: https://<host>/v1
API key: $ALLROUTER_TOKEN, or any string when ALLROUTER_TOKEN is unset
Model: local/<local-model>, an alias id such as gpt-4o, or a TrustedRouter model
```

## 6. Routing Cheatsheet

Pin local:

```json
{"model":"local/llama3.2","messages":[{"role":"user","content":"ping"}]}
```

Pin local with provider directives:

```json
{"model":"llama3.2","provider":{"only":["local"]},"messages":[{"role":"user","content":"ping"}]}
```

Force TrustedRouter:

```json
{"model":"anthropic/claude-haiku-4.5","provider":{"order":["anthropic"]},"messages":[{"role":"user","content":"ping"}]}
```

Alias cloud id to local model:

```bash
allrouter -local-url http://127.0.0.1:11434 \
  -alias gpt-4o=llama3.2 \
  -savings-reference gpt-4o
```

Allow unmapped local-native ids to burst with a fallback model:

```bash
allrouter -local-url http://127.0.0.1:11434 \
  -tr-api-key "$TRUSTEDROUTER_API_KEY" \
  -burst-fallback-model openai/gpt-4o-mini
```

Read routing:

```bash
curl -fsS -H "Authorization: Bearer $ALLROUTER_TOKEN" "$ALLROUTER_HOST/stats"
```

Response headers:

```http
X-AllRouter-Route: local
X-AllRouter-Reason: policy
```

Reasons: `policy`, `forced`, `burst-full`, `burst-error`, `burst-slow`.
