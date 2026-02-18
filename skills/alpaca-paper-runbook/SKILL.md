---
name: alpaca-paper-runbook
description: Configure and operate helix-tui against Alpaca paper trading endpoints, including credentials, runtime flags, and basic adapter troubleshooting. Use when asked to connect autonomous or manual runs to Alpaca paper mode.
---

# Alpaca Paper Runbook

Use Alpaca paper mode for broker-backed testing without live capital.

## Set Credentials
- Set:
  - `APCA_API_KEY_ID`
  - `APCA_API_SECRET_KEY`
- Pass flags explicitly when needed:
  - `-alpaca-key`
  - `-alpaca-secret`

## Start in Paper Mode
- Use:
  - `-broker=alpaca-paper`
- Keep autonomous safety flags enabled during first runs:
  - `-mode=assist` or `-mode=auto -dry-run`

Example:

```bash
go run ./cmd/helix -broker=alpaca-paper -mode=assist
```

## Common Runtime Checks
- Confirm startup does not fail with missing key/secret errors.
- Confirm `sync` succeeds and account/positions load.
- Confirm order placement path works from TUI command (`buy` / `sell`) before autonomous auto-execution.

## Known Scaffold Limits
- Treat quote retrieval in Alpaca adapter as incomplete unless implemented in code.
- Treat websocket trade updates in Alpaca adapter as incomplete unless implemented in code.
- Use paper broker adapter for full autonomous loop testing when quote/ws functions are required.

## Troubleshooting
- If authentication errors occur:
  - verify key/secret pair
  - verify paper endpoint mode is used
- If autonomous mode does not execute:
  - check events for `agent_intent_rejected` and risk gate violations
  - verify symbol is in allowlist (`-allow=...`)
- If no intents appear:
  - lower `-agent-move-pct`
  - increase watchlist coverage

## Validation
- Run:
  - `go test ./...`
  - `go build ./cmd/helix`
