# helix-tui

Go CLI + TUI trading app scaffold with a safety-first architecture:

- Broker adapter boundary (`paper` now, `alpaca-paper` scaffolded)
- Deterministic risk gate between intent and execution
- State engine with event log and reconciliation
- Autonomous agent runtime modes (`manual`, `assist`, `auto`)
- Terminal UI command cockpit (Bubble Tea)

This repo is a foundation for the architecture discussed in your shared thread, not a production bot.

## Architecture

- `cmd/helix`
  - Application entrypoint and CLI flags
- `internal/domain`
  - Core types and interfaces (`Broker`, `Agent`, orders, positions, snapshots)
- `internal/engine`
  - Trading engine + risk gate
- `internal/broker/paper`
  - In-memory paper broker with instant fills
- `internal/broker/alpaca`
  - Official Alpaca Go SDK adapter (`alpaca-trade-api-go/v3`)
- `internal/tui`
  - Bubble Tea terminal UI

## Prerequisites

- Go 1.22+

## Run

```bash
go mod tidy
go run ./cmd/helix
```

## Config File (TOML)

The app can load runtime settings from a TOML file using `github.com/pelletier/go-toml/v2`.

- Default path: `config.toml` in the project root (auto-loaded if present)
- Override path: `-config=path/to/file.toml`
- Example template: `config.example.toml`

Config precedence is:

- built-in defaults
- TOML file
- environment variables (`APCA_API_KEY_ID`, `APCA_API_SECRET_KEY`, `APCA_API_DATA_URL`)
- CLI flags

Windows (PowerShell) quick start:

```powershell
Copy-Item config.example.toml config.toml
go run ./cmd/helix -config=config.toml
```

Useful flags:

```bash
go run ./cmd/helix \
  -config=config.toml \
  -broker=paper \
  -max-trade=5000 \
  -max-day=20000 \
  -allow=AAPL,MSFT,TSLA,NVDA \
  -mode=manual
```

Alpaca paper mode:

```bash
go run ./cmd/helix \
  -config=config.toml \
  -broker=alpaca-paper \
  -alpaca-feed=iex \
  -alpaca-key=$APCA_API_KEY_ID \
  -alpaca-secret=$APCA_API_SECRET_KEY \
  -mode=assist
```

Alpaca credentials with keyring (recommended):

```bash
go run ./cmd/helix \
  -config=config.toml \
  -broker=alpaca-paper \
  -alpaca-feed=iex \
  -use-keyring \
  -save-keyring \
  -keyring-service=helix-tui \
  -keyring-user=alpaca-paper
```

Windows (PowerShell) credential setup:

1. Add your Alpaca paper credentials to the current shell:

```powershell
$env:APCA_API_KEY_ID = "YOUR_KEY_ID"
$env:APCA_API_SECRET_KEY = "YOUR_SECRET_KEY"
```

1. Run once with keyring save enabled to store credentials in Windows Credential Manager:

```powershell
go run ./cmd/helix -broker=alpaca-paper -alpaca-feed=iex -use-keyring -save-keyring -mode=assist
```

1. Future runs can omit `-alpaca-key` / `-alpaca-secret` and environment variables:

```powershell
go run ./cmd/helix -broker=alpaca-paper -alpaca-feed=iex -use-keyring -mode=assist
```

Optional: persist environment variables for new terminals (if you still want env-based auth):

```powershell
setx APCA_API_KEY_ID "YOUR_KEY_ID"
setx APCA_API_SECRET_KEY "YOUR_SECRET_KEY"
```

Open a new terminal after `setx` for changes to take effect.

Headless autonomous loop:

```bash
go run ./cmd/helix \
  -broker=paper \
  -mode=auto \
  -headless \
  -watchlist=AAPL,MSFT,TSLA \
  -agent-interval=10s \
  -agent-qty=1 \
  -agent-move-pct=0.01 \
  -agent-max-intents=1
```

## TUI Commands

- `buy <SYM> <QTY>`
- `sell <SYM> <QTY>`
- `cancel <ORDER_ID>`
- `flatten`
- `sync`
- `help`
- `q`

## Runtime Modes

- `manual`: agent intents are ignored; only manual commands trade.
- `assist`: agent generates intents but does not execute, and logs `agent_intent_needs_approval` events.
- `auto`: agent intents are executed automatically through the same risk gate as manual orders.

`-dry-run` works with autonomous modes and logs intents without submitting orders.

## Safety Defaults

- Symbol allowlist
- Max notional per trade
- Max daily notional

These checks are enforced in `internal/engine/risk.go`.

## Notes

- The paper broker fills immediately at the current mock quote.
- The Alpaca adapter uses the official SDK client for account/positions/orders plus market data latest quote and trade update streaming.
- Alpaca market data permissions/entitlements still apply to quote availability.
- Alpaca quote feed defaults to `iex` (override via `-alpaca-feed`).
- When `-use-keyring` is enabled, missing Alpaca credentials are loaded from OS keyring; provided credentials can be stored with `-save-keyring`.
- The built-in agent is a conservative heuristic (`internal/agent/heuristic`) that reacts to sampled price moves.
