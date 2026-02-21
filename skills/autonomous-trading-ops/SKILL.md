---
name: autonomous-trading-ops
description: Operate helix-tui autonomous sessions with config-first safe defaults, TUI monitoring, and controlled escalation from manual to auto execution.
---

# Autonomous Trading Ops

Use this skill for runtime operations and autonomous execution control.

## Current Runtime Assumptions

- Runtime broker is Alpaca.
- Use `[alpaca].env = "paper"` for safety unless explicitly asked to use live.
- Runtime behavior is configured in `config.toml` (not legacy execution flags).

## Safe Startup Sequence

1. Start with:
   - `mode = "manual"` or `mode = "assist"`
   - `[alpaca].env = "paper"`
2. If validating `auto`, keep:
   - `mode = "auto"`
   - `[agent].dry_run = true`
3. Remove dry-run only after event flow and order behavior are acceptable.

Run:

```bash
go run ./cmd/helix -config=config.toml
```

Headless:

```bash
go run ./cmd/helix -config=config.toml -headless
```

## Tuning Knobs (config.toml)

- `[agent].interval`
- `[agent].max_intents`
- `[agent].min_gain_pct`
- `[agent].sync_timeout`
- `[agent].order_timeout`
- `[agent].dry_run`
- `[agent.low_power].*`
- `[risk].max_trade_notional`
- `[risk].max_day_notional`

## Monitor in TUI

Watch:

- account totals in header (`Cash`, `Buying Power`, `Equity`)
- `Open Orders` table for stuck/replaced orders
- `Logs` tab for `agent_cycle_error`, `agent_intent_rejected`, `order_placed`, `trade_update`
- `System` tab for request counters and persistence health
- `Strategy` tab if strategy mode is enabled

## Session Commands

- `sync`
- `cancel <ORDER_ID|ORDER_ID_PREFIX|#ROW>`
- `flatten`
- `watch list|add|remove|sync`
- `strategy run|status|approve|reject|archive`

## Validation

```bash
go test ./...
go build ./cmd/helix
```
