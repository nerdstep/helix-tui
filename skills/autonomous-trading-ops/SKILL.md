---
name: autonomous-trading-ops
description: Operate helix-tui in manual, assist, or auto mode with safe defaults, monitoring, and command-line tuning. Use when asked to run autonomous agent sessions, monitor agent activity in the TUI, or configure runtime flags for autonomous trading behavior.
---

# Autonomous Trading Ops

Run autonomous sessions with explicit mode and monitoring choices.

## Use Safe Startup Sequence
- Start in `paper` broker mode.
- Start in `assist` or `auto` with `-dry-run` first.
- Remove `-dry-run` only after reviewing event flow and trade intents.

Example:

```bash
go run ./cmd/helix -broker=paper -mode=auto -dry-run -watchlist=AAPL,MSFT,TSLA -agent-interval=5s
```

## Choose Runtime Shape
- Use TUI mode (omit `-headless`) when human monitoring is required.
- Use `-headless` for unattended execution with periodic console summaries.

Example headless run:

```bash
go run ./cmd/helix -broker=paper -mode=auto -headless -watchlist=AAPL,MSFT -agent-interval=10s
```

## Tune Autonomous Behavior
- Adjust `-agent-interval` to control cycle frequency.
- Adjust `-agent-max-intents` to cap per-cycle execution.
- Adjust `-agent-qty` to control order size.
- Adjust `-agent-move-pct` to control intent trigger threshold.

## Monitor in TUI
- Watch account line (`Cash`, `BuyingPower`, `Equity`) for drift and P/L effects.
- Watch `Open Orders` for stuck or repeated submissions.
- Watch `Recent Events` for:
  - `agent_runner_start`
  - `agent_intent_needs_approval`
  - `agent_intent_executed`
  - `agent_intent_rejected`
  - `agent_cycle_error`
  - `order_placed`
  - `trade_update`

## Operator Commands During Session
- Use `sync` for immediate reconciliation.
- Use `cancel <ORDER_ID>` to stop a pending order.
- Use `flatten` to exit positions quickly.
- Use `q` to quit TUI.

## Validation
- Run:
  - `go test ./...`
  - `go build ./cmd/helix`
