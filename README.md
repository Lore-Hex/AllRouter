# HybridRouter

[![CI](https://github.com/Lore-Hex/HybridRouter/actions/workflows/ci.yml/badge.svg)](https://github.com/Lore-Hex/HybridRouter/actions/workflows/ci.yml)
[![License: Elastic-2.0](https://img.shields.io/badge/License-Elastic--2.0-blue.svg)](LICENSE)

HybridRouter is one endpoint for local and cloud models. Keep routine work on your own hardware, send selected jobs to the cloud, or automatically move overflow to [TrustedRouter](https://trustedrouter.com) when local capacity is full, slow, or unavailable.

It speaks both the OpenAI and Anthropic APIs. Existing tools can choose local or cloud per request, while HybridRouter enforces concurrency, spend, privacy, and fallback policy in one place.

```text
brew tap Lore-Hex/homebrew-tap && brew install hybridrouter
export TRUSTEDROUTER_API_KEY="tr_..." # optional: enables cloud passthrough/bursting
hybridrouter
Point your tools at http://localhost:8383/v1
```

Alternates: `go install github.com/Lore-Hex/HybridRouter/cmd/hybridrouter@latest`, download a binary from the [latest release](https://github.com/Lore-Hex/HybridRouter/releases/latest), or run the Docker image you build from this repo.

## BackupRouter for coding agents

BackupRouter is a HybridRouter preset for Claude Code Desktop, Claude Code CLI,
and Codex. The client keeps requesting its configured model first.
TrustedRouter retries that model across available providers, then falls back to
Kimi K3 and GLM 5.2 if the original route is unavailable.

Start the gateway:

```bash
export TRUSTEDROUTER_API_KEY="tr_..."
mkdir -p "$HOME/.config/hybridrouter"
chmod 700 "$HOME/.config/hybridrouter"
if [ ! -s "$HOME/.config/hybridrouter/desktop-token" ]; then
  (umask 077 && openssl rand -hex 24 > "$HOME/.config/hybridrouter/desktop-token")
fi
export HYBRID_TOKEN="$(cat "$HOME/.config/hybridrouter/desktop-token")"
hybridrouter -listen 127.0.0.1:8383 -preset backuprouter -no-autodetect
```

Keep that process running while a coding client is connected. For daily Desktop
use, run it under your operating system's process supervisor. Keep
`TRUSTEDROUTER_API_KEY` in the HybridRouter process only.

### Claude Code Desktop

[Claude Code Desktop](https://code.claude.com/docs/en/desktop) does not use
`ANTHROPIC_BASE_URL` or Claude Code CLI's `settings.json` for third-party
inference. Configure its
[gateway connection](https://claude.com/docs/third-party/claude-desktop/gateway)
directly:

1. Open **Help > Troubleshooting > Enable Developer Mode** and allow Claude to
   restart.
2. Open **Developer > Configure Third-Party Inference**.
3. Set the connection fields:

   | Field | Value |
   | --- | --- |
   | Inference provider | `Gateway` |
   | Gateway base URL | `http://127.0.0.1:8383` |
   | Credential kind | `Static API key` |
   | Gateway API key | contents of `~/.config/hybridrouter/desktop-token` |
   | Gateway auth scheme | `bearer` |
   | Model discovery | Off |
   | Model ID | `anthropic/claude-sonnet-5` |
   | Display name | `BackupRouter` |
   | Tier alias | `sonnet` |
   | Default for tier | On |

4. Save and apply the changes. Start a **new local Code session** and select
   **BackupRouter**.

Claude Desktop requires an Anthropic-prefixed model ID. This request still goes
through TrustedRouter, not your Anthropic subscription, and HybridRouter adds
the configured Kimi and GLM recovery routes.

Do not paste `TRUSTEDROUTER_API_KEY` into Claude Desktop. The Desktop app gets a
loopback-only bearer token; HybridRouter holds the upstream credential and
performs routing. Existing Anthropic-backed conversations do not change
providers retroactively, so their old usage-limit banner can remain visible.

Verify the local service before opening a new session:

```bash
curl -fsS \
  -H "Authorization: Bearer $HYBRID_TOKEN" \
  http://127.0.0.1:8383/stats
```

If Desktop reports `401`, its Gateway API key does not match `HYBRID_TOKEN`. If
it cannot connect, confirm HybridRouter is still listening on
`127.0.0.1:8383` and that the configured base URL does not end in `/v1`.
Third-party inference applies to local Desktop sessions; Claude's remote
environments and Remote Control are not available through this gateway.

### Claude Code CLI

Point Claude Code CLI at the same gateway:

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8383"
export ANTHROPIC_AUTH_TOKEN="$HYBRID_TOKEN"
export ANTHROPIC_MODEL="anthropic/claude-sonnet-5"
export ANTHROPIC_SMALL_FAST_MODEL="anthropic/claude-haiku-4.5"
claude
```

The resulting order is:

```text
Claude requested by Claude Code
  -> moonshotai/kimi-k3
  -> z-ai/glm-5.2
```

Change the backup order with repeatable flags:

```bash
hybridrouter -preset backuprouter -no-autodetect \
  -backup-model z-ai/glm-5.2 \
  -backup-model moonshotai/kimi-k3
```

Or open [http://127.0.0.1:8383/ui](http://127.0.0.1:8383/ui). Start with a Balanced, Fast, Economy, Zero retention, or End to end encrypted profile, then customize the model roles:

- **Primary model** handles planning, coding, and difficult work.
- **Small task model** handles routine search, summaries, and background work through `ANTHROPIC_SMALL_FAST_MODEL`.
- **Recovery models** are tried in order when the requested model is unavailable.

The model picker searches the live TrustedRouter chat catalog and exposes price, context length, privacy tier, open-weight status, managed routes, and provider coverage. It deliberately excludes embedding-only, hidden, and internal models. Search across all models or narrow to managed pools, zero-retention routes, open-weight models, million-token context, and models under $1 per million output tokens.

Saving applies the recovery chain to new requests immediately and persists it in `$XDG_CONFIG_HOME/hybridrouter/config.json` or `~/.config/hybridrouter/config.json`. The client tabs generate model-specific setup for Claude Code, Codex CLI, and the ChatGPT Desktop Codex workspace. Claude Code receives both the primary and small model; Codex uses the primary model. Explicit `-backup-model` flags and `HYBRID_BACKUP_MODELS` override the saved dashboard route at startup. Set `HYBRID_TOKEN` before launch to protect the dashboard and configuration API with the same bearer token used by connected clients.

An explicit `models` array in a request takes precedence over the configured backup list. HybridRouter sends one request to TrustedRouter; provider and model rollover happens inside TrustedRouter's attested gateway, so client retries do not create duplicate inference calls.

### Codex CLI and ChatGPT Desktop

Codex uses the Responses API. Add this user-level provider configuration to `~/.codex/config.toml`:

```toml
model = "trustedrouter/auto"
model_provider = "hybridrouter"

[model_providers.hybridrouter]
name = "HybridRouter"
base_url = "http://127.0.0.1:8383/v1"
env_key = "HYBRID_TOKEN"
wire_api = "responses"
```

For Codex CLI, export the local bearer token and launch normally:

```bash
export HYBRID_TOKEN="$(cat "$HOME/.config/hybridrouter/desktop-token")"
codex
```

The ChatGPT Desktop Codex workspace shares the same `~/.codex/config.toml`. Open **Settings > Configuration > Open config.toml**, add the provider block, and make the token available to GUI-launched apps before restarting:

```bash
launchctl setenv HYBRID_TOKEN \
  "$(cat "$HOME/.config/hybridrouter/desktop-token")"
```

This changes the provider used by the Codex workspace; ordinary ChatGPT conversations do not support replacing OpenAI's model provider with a local endpoint.

Codex has one selected model rather than Claude Code's separate primary and small-task environment variables. HybridRouter still injects the saved recovery list into `/v1/responses` requests. The dashboard client tabs generate the current model-specific Claude or Codex configuration.

## Private by design

HybridRouter is local-first, so most requests never leave your machine. The privacy story is what makes the *overflow* safe too.

When a request bursts, the default target is **TrustedRouter — an end-to-end encrypted AI gateway that runs inside an attested Trusted Execution Environment (TEE)**. The gateway is cryptographically attested to match its open-source code, so **no one — not even TrustedRouter's own operators — can read your prompts or completions**. There are no prompt/output logs, the control plane holds metadata only, and the router **fails closed if attestation can't be verified**.

**Beyond ZDR — encrypted all the way to the model.** Zero-data-retention means a provider promises not to *keep* your prompt; it can still *see* it. TrustedRouter goes further. Pin sensitive traffic to a privacy route and the guarantee travels with the request:

- `trustedrouter/zdr` — zero-data-retention providers only.
- `trustedrouter/e2e` — **end-to-end encrypted to confidential-compute (encrypted) LLM endpoints**, so the prompt stays encrypted through the gateway *and* at the model itself. This is the tier other routers don't offer.

**Verifiable, not "trust us."** TrustedRouter publishes the running source commit, image reference, image digest, and attestation path on a public [trust page](https://trust.trustedrouter.com) — you can check what code handled your request. `hybridrouter` speaks the same OpenAI/Anthropic APIs as everything else, but a burst to TrustedRouter lands somewhere you can cryptographically verify, unlike a black-box router (OpenRouter and other intermediaries) that can quietly log prompts.

You stay in control of *whether* traffic leaves at all — see [Cloud Controls](#cloud-controls) — and TrustedRouter guarantees it's private *when* it does.

## Routing Contract

| Request directive or condition | Behavior |
| --- | --- |
| No directive | Local first when `-local-url` is configured; TrustedRouter when local is absent. |
| `-alias gpt-4o=llama3.2` and request `model: "gpt-4o"` | Local first; forwards to local as `llama3.2`, but any burst uses the original `gpt-4o` id. |
| `model: "local/<name>"` | Forced local; forwards to local as `<name>`. |
| `provider.only: ["local"]` | Forced local; strips `provider` before local forwarding. |
| `provider.order: ["local"]` | Local preference, not a hard pin; can still burst when the model is burst-capable. |
| Any non-local provider in `provider.only` or `provider.order` | Forced TrustedRouter. |
| Local-native id with no `/`, no alias, and no fallback model | Effectively local-only; local full returns `429`, and local errors surface without a doomed burst. |
| Local-native id with `-burst-fallback-model` set | Can burst; HybridRouter substitutes the fallback model only in the burst body. |
| Local semaphore full | Bursts to TrustedRouter when not forced, TR is configured, and the model is burst-capable; otherwise returns `429`. |
| Local connect error, `429`, `5xx`, or model-missing `404` | Bursts to TrustedRouter when `-burst-on-error=true`, not forced, TR is configured, and the model is burst-capable. |
| Local headers arrive but the first body byte exceeds `-local-slow-after` | Bursts to TrustedRouter when the deadline is enabled, not forced, cloud egress is allowed, TR is configured, and the model is burst-capable. |
| `-preset backuprouter` | Keeps the requested model first, then adds Kimi K3 and GLM 5.2 as ordered TrustedRouter fallbacks. |
| Request contains `models` | The request's explicit fallback order wins; configured backup models are not injected. |
| `/v1/messages` with an alias or `local/<name>` | Local-capable: translates Anthropic Messages to local OpenAI chat/completions, then translates the response (and streaming events) back. Bursts send the original Anthropic body. |
| `/v1/messages` with an unmapped Claude cloud id | Raw TrustedRouter passthrough, preserving the Anthropic body. |
| `/v1/responses` | TrustedRouter-only raw passthrough; local-forced requests return `400`, local-only mode returns `501`, and upstream `404` maps to a Hybrid `501`. |

## Configuration

| Flag | Env | Default |
| --- | --- | --- |
| `-listen` | `HYBRID_LISTEN` | `:8383` |
| `-local-url` | `HYBRID_LOCAL_URL` | `""` |
| `-tr-api-key` | `TRUSTEDROUTER_API_KEY` | `""` |
| `-tr-base-url` | `HYBRID_TR_BASE_URL` | `https://api.quillrouter.com/v1` |
| `-tr-catalog-url` | `HYBRID_TR_CATALOG_URL` | `https://trustedrouter.com/v1` |
| `-local-max-concurrency` | `HYBRID_LOCAL_MAX_CONCURRENCY` | `4` |
| `-local-queue-wait` | `HYBRID_LOCAL_QUEUE_WAIT` | `0s` |
| `-local-slow-after` | `HYBRID_LOCAL_SLOW_AFTER` | `0s` |
| `-burst-on-error` | `HYBRID_BURST_ON_ERROR` | `true` |
| `-burst-fallback-model` | `HYBRID_BURST_FALLBACK_MODEL` | `""` |
| `-preset` | `HYBRID_PRESET` | `""`; supports `backuprouter` |
| `-backup-model` | `HYBRID_BACKUP_MODELS` | repeatable; BackupRouter defaults to Kimi K3, then GLM 5.2 |
| `-alias from=to` | `HYBRID_ALIASES=a=b,c=d` | `""` |
| `-savings-reference` | `HYBRID_SAVINGS_REFERENCE` | `""` |
| `-state-file` | `HYBRID_STATE_FILE` | `$XDG_STATE_HOME/hybridrouter/state.json` or `~/.hybridrouter/state.json`; `""` disables |
| `-config-file` | `HYBRID_CONFIG_FILE` | `$XDG_CONFIG_HOME/hybridrouter/config.json` or `~/.config/hybridrouter/config.json`; `""` disables UI persistence |
| `-cloud` | `HYBRID_CLOUD` | `auto` |
| `-max-cloud-spend` | `HYBRID_MAX_CLOUD_SPEND` | `0` |
| `-sse-batch-window` | `HYBRID_SSE_BATCH_WINDOW` | `0s` |
| `-sse-batch-max-bytes` | `HYBRID_SSE_BATCH_MAX_BYTES` | `4096` |
| `-no-autodetect` | none | `false` |
| `-version` | none | `false` |
| `-token` | `HYBRID_TOKEN` | `""` |

When `-local-url` is unset, HybridRouter probes `OLLAMA_HOST`, Ollama, LM Studio, llama.cpp, and vLLM on common localhost ports. If no local server is found, `TRUSTEDROUTER_API_KEY` enables pure cloud passthrough; without either, startup prints an actionable error. Use `-no-autodetect` to disable local probing. Set `HYBRID_TOKEN` whenever the proxy is reachable outside localhost. Auth accepts either `Authorization: Bearer <token>` or `x-api-key: <token>`.

Aliases map cloud-facing ids to local model ids. For example, `-alias gpt-4o=qwen2.5-coder:32b` lets tools request `gpt-4o`; local receives `qwen2.5-coder:32b`, while bursts still send `gpt-4o`.

`-sse-batch-window` coalesces streamed chat-completions content chunks to cut egress bytes — each token otherwise spends ~150–250 bytes of SSE/JSON framing on a few bytes of content. It's off by default (zero added latency on localhost); set it (e.g. `-sse-batch-window 40ms`) when HybridRouter is exposed over ngrok or a WAN, where per-byte egress matters. The first token always flushes immediately so time-to-first-token is unchanged, and reasoning/tool-call frames are never merged.

## Claude Code On Your GPU

Claude Code and Anthropic SDKs can also use a local model first. Map the Claude model id your tool sends to a local OpenAI-compatible model:

```bash
export TRUSTEDROUTER_API_KEY="tr_..."
hybridrouter -local-url http://127.0.0.1:11434 \
  -tr-api-key "$TRUSTEDROUTER_API_KEY" \
  -alias anthropic/claude-haiku-4.5=qwen2.5-coder:32b

export ANTHROPIC_BASE_URL="http://127.0.0.1:8383"
export ANTHROPIC_AUTH_TOKEN="${HYBRID_TOKEN:-hybridrouter-local}"
export ANTHROPIC_MODEL="anthropic/claude-haiku-4.5"
```

Use the exact model id your Claude Code configuration sends on the left side of `-alias`. The local leg translates `/v1/messages` into `/v1/chat/completions`, including text, tools, tool results, and streaming. When local is full or fails and cloud egress is allowed, HybridRouter bursts the original Anthropic request body to TrustedRouter.

Coding agents send your source, secrets, and internal context in every prompt. Running them local-first keeps that on your machine, and bursts land on TrustedRouter's [attested gateway](#private-by-design). Use `trustedrouter/e2e` or a provider directive when the downstream model must also run in confidential compute.

## Savings

HybridRouter keeps an honest savings meter in `/stats` and `X-Hybrid-Saved-USD`. Local tokens are priced only as a labeled counterfactual using TrustedRouter catalog prices. The reference is chosen in order: the alias key for aliased requests, the requested TrustedRouter-known model, `-savings-reference`, then tokens-only with no dollars. HybridRouter never invents a price when the catalog has no price anchor.

For local model names that are not cloud ids, pair the local alias with an explicit savings reference:

```bash
hybridrouter -local-url http://127.0.0.1:11434 \
  -tr-api-key "$TRUSTEDROUTER_API_KEY" \
  -alias gpt-4o=llama3.2 \
  -savings-reference gpt-4o
```

Cloud spend is priced from the actual model id returned by the cloud response when that model exists in the TrustedRouter catalog. Unpriced cloud usage still counts tokens in stats but counts `$0` toward the spend cap.

## Cloud Controls

`-cloud=auto|explicit|off` controls cloud egress. `auto` preserves normal bursting. `explicit` disables automatic bursts, so local-full requests return `429` and local errors surface, while requests that explicitly name a non-local provider can still go out. `off` disables the cloud upstream entirely; explicit cloud requests fail closed with `cloud disabled by -cloud=off`.

Send `SIGHUP` to toggle runtime cloud egress between the configured mode and `off`. `/stats` reports the effective mode.

`-max-cloud-spend <usd>` sets a per-UTC-day cloud spend cap. Once priced cloud spend reaches the cap, all cloud sends return `429 cloud_budget_exhausted` with `Retry-After` set to seconds until UTC midnight. Unpriced cloud usage honestly counts as `$0` toward the cap.

These controls decide *whether* a prompt leaves your machine. When one does leave to the default TrustedRouter upstream, it is [end-to-end encrypted and handled by an attested TEE](#private-by-design) — so you get both: control over egress, and a private, verifiable destination when egress happens.

## Bursting To Other Clouds

`-tr-base-url` can point at any bearer-keyed OpenAI-compatible `/v1` base URL, including OpenRouter, Together, Groq, or your own upstream. TrustedRouter is only the default. Savings/pricing features use the TrustedRouter catalog.

Note the tradeoff: TrustedRouter is the default because it is [end-to-end encrypted, attested, and log-free](#private-by-design) with encrypted endpoints beyond ZDR. Generic OpenAI-compatible routers such as OpenRouter are black boxes that can log your prompts — pointing `-tr-base-url` at one trades away that privacy guarantee. Keep the default when prompts matter.

If that upstream does not implement `/v1/messages` or `/v1/responses`, HybridRouter maps cloud passthrough `404`s to a clean `501 endpoint_not_supported` Hybrid error envelope. Aliased local `/v1/messages` requests do not require the burst upstream to support Anthropic Messages.

## Endpoints

| Endpoint | Mode |
| --- | --- |
| `GET /healthz` | Local health metadata. |
| `GET /stats` | Hybrid counters; bearer-protected when `HYBRID_TOKEN` is set. |
| `GET /ui` | Read-only savings dashboard; bearer-protected when `HYBRID_TOKEN` is set. |
| `GET /metrics` | Prometheus text metrics; bearer-protected when `HYBRID_TOKEN` is set. |
| `GET /v1/models` | Merged local and TrustedRouter model list. |
| `POST /v1/chat/completions` | Local-capable, burst-capable. |
| `POST /v1/embeddings` | Local-capable, burst-capable. |
| `POST /v1/messages` | Local-capable Anthropic Messages translation; raw TrustedRouter passthrough for cloud. |
| `POST /v1/responses` | TrustedRouter-only raw passthrough. |

## Responses

Non-streaming JSON responses get a top-level Hybrid block:

```json
{
  "hybrid": {
    "route": "local",
    "reason": "policy"
  }
}
```

Every routed response also includes:

```http
X-Hybrid-Route: local
X-Hybrid-Reason: policy
```

Routes are `local` or `trustedrouter`. Reasons are `policy`, `forced`, `burst-full`, `burst-error`, or `burst-slow`. Streaming responses pass through byte-for-byte and use headers only.

## Stats

`GET /stats` reports `in_flight_local`, `local_capacity`, `bursts_full`, `bursts_error`, `bursts_slow`, `bursts_skipped_unmapped`, `forced_local`, `forced_tr`, `requests_total`, `cloud_mode`, `cloud_blocked_budget`, `cloud_blocked_mode`, `savings`, global `routes`, `endpoint_routes` for `chat_completions`, `embeddings`, `messages`, and `responses`, and a bounded `recent` feed of the last routing decisions.

## Dashboards

Open `http://127.0.0.1:8383/ui` for the read-only savings odometer and live routing feed. If `HYBRID_TOKEN` is set, serve it with the same bearer token used for `/stats`.

Prometheus can scrape `GET /metrics`, which exposes `hybrid_requests_total`, `hybrid_in_flight_local`, route, burst, savings, token, unknown-usage, cloud-spend, and cloud-blocked metrics. Import [docs/grafana-dashboard.json](docs/grafana-dashboard.json) for a starter Grafana dashboard with savings, local-vs-cloud rate, in-flight, and cloud-spend panels.

## Setup

Use [docs/SETUP.md](docs/SETUP.md) for a copy-paste setup reference. Run `scripts/smoke.sh` to verify a local install against Ollama. Agent harnesses can use [skills/hybridrouter-setup/SKILL.md](skills/hybridrouter-setup/SKILL.md) as an interactive setup skill.

## License

Elastic License 2.0. You may use, copy, modify, and redistribute HybridRouter, but you may not offer it to third parties as a managed service.
