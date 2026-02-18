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
- Use keyring for persisted credentials:
  - `-use-keyring`
  - `-save-keyring`
  - optional: `-keyring-service`, `-keyring-user`

## Start in Paper Mode
- Use:
  - `-broker=alpaca-paper`
  - `-alpaca-feed=iex` (default feed)
- Keep autonomous safety flags enabled during first runs:
  - `-mode=assist` or `-mode=auto -dry-run`

Example:

```bash
go run ./cmd/helix -broker=alpaca-paper -alpaca-feed=iex -mode=assist
```

## Common Runtime Checks
- Confirm startup does not fail with missing key/secret errors.
- Confirm `sync` succeeds and account/positions load.
- Confirm order placement path works from TUI command (`buy` / `sell`) before autonomous auto-execution.

## Known Scaffold Limits
- Quote retrieval and trade update streaming are wired through the official Alpaca Go SDK.
- Market data availability depends on account entitlements/feed availability (`iex` vs `sip`).
- If quote retrieval fails in paper mode, test with `paper` broker adapter to isolate entitlement vs application issues.

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
