---
name: risk-incident-response
description: Execute risk-first incident response for abnormal autonomous behavior, including containment, exposure neutralization, diagnosis, and controlled recovery.
---

# Risk Incident Response

Contain first, analyze second, resume last.

## Immediate Containment

1. Stop autonomous operation:
   - quit TUI (`q`) or terminate headless process
2. Preserve logs:
   - `logs/helix.log`
   - relevant TUI/system events
3. Do not restart directly in `mode = "auto"` without dry-run.

## Neutralize Exposure

Restart in monitored mode (`manual` or `assist`) and run:

- `sync`
- `cancel <ORDER_ID|ORDER_ID_PREFIX|#ROW>` for risky open orders
- `flatten` if positions should be exited

Verify state after each command.

## Diagnose

Inspect Logs/System tab for:

- `agent_cycle_error`
- `agent_intent_rejected`
- repeated `order_placed` / `trade_update`
- compliance and persistence errors

Review config controls:

- `[agent].interval`
- `[agent].max_intents`
- `[agent].min_gain_pct`
- `[agent].dry_run`
- `[risk].max_trade_notional`
- `[risk].max_day_notional`
- `[compliance].*`

## Controlled Recovery

Progress in stages:

1. `mode = "assist"`
2. `mode = "auto"` with `[agent].dry_run = true`
3. full `auto` only after stable cycles and expected event patterns

## Post-Incident Hardening

- reduce `[agent].max_intents`
- increase `[agent].interval`
- tighten risk/compliance thresholds
- narrow watchlist
- document incident + corrective changes

## Validation

```bash
go test ./...
go build ./cmd/helix
```
