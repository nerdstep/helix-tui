---
name: alpaca-paper-runbook
description: Configure and operate helix-tui in Alpaca paper environment, including credentials/keyring setup, connectivity verification, and common troubleshooting.
---

# Alpaca Paper Runbook

Use this skill when setting up or troubleshooting Alpaca paper operations.

## Current Runtime Assumptions

- Runtime broker is Alpaca.
- Paper vs live is controlled by `[alpaca].env`.
- Preferred paper default:
  - `[alpaca].env = "paper"`
  - `[alpaca].feed = "iex"`

## Credential Sources

Credentials can come from:

- `config.toml` (`[alpaca].api_key`, `[alpaca].api_secret`)
- environment (`APCA_API_KEY_ID`, `APCA_API_SECRET_KEY`)
- OS keyring (`[keyring].use = true`)

For LLM mode/strategy also support:

- `OPENAI_API_KEY`
- `[agent.llm].api_key`
- keyring via same service/user

## Basic Bring-Up

1. Copy template:

```bash
cp config.example.toml config.toml
```

1. Set:
   - `[alpaca].env = "paper"`
   - `mode = "manual"` (or `assist`)
2. Run:

```bash
go run ./cmd/helix -config=config.toml
```

3. Verify sync/account load:
   - header values populate
   - `sync` command succeeds
   - no credential/auth errors in Logs tab

## Troubleshooting

- Auth failures:
  - verify paper key/secret pair
  - verify `[alpaca].env = "paper"`
  - verify no stale env vars overriding config
- Missing/poor quotes:
  - confirm `[alpaca].feed`
  - check market data entitlements
- Autonomous issues:
  - inspect `agent_intent_rejected` details/rejection_reason
  - verify symbol is present in watchlist (watchlist is allowlist)

## Validation

```bash
go test ./...
go build ./cmd/helix
```
