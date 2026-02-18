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
  - Alpaca REST scaffold (quotes + ws not fully wired yet)
- `internal/tui`
  - Bubble Tea terminal UI

## Prerequisites

- Go 1.22+

## Run

```bash
go mod tidy
go run ./cmd/helix
```

Useful flags:

```bash
go run ./cmd/helix \
  -broker=paper \
  -max-trade=5000 \
  -max-day=20000 \
  -allow=AAPL,MSFT,TSLA,NVDA \
  -mode=manual
```

Alpaca paper mode (scaffold):

```bash
go run ./cmd/helix \
  -broker=alpaca-paper \
  -alpaca-key=$APCA_API_KEY_ID \
  -alpaca-secret=$APCA_API_SECRET_KEY \
  -mode=assist
```

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
- The Alpaca adapter wires account/positions/open orders/place/cancel paths.
- Alpaca quote and trade update websocket handling are intentionally left as explicit TODOs so the boundaries stay clean.
- The built-in agent is a conservative heuristic (`internal/agent/heuristic`) that reacts to sampled price moves.
