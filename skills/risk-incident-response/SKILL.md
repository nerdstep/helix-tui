---
name: risk-incident-response
description: Execute risk-first incident response for helix-tui autonomous sessions, including immediate containment, order/position neutralization, and controlled recovery. Use when autonomous behavior appears abnormal, excessive, or unsafe.
---

# Risk Incident Response

Contain first, analyze second, resume last.

## Immediate Containment
- Stop autonomous execution:
  - quit TUI (`q`) or terminate headless process
- Preserve logs/events for diagnosis.
- Do not restart in `auto` until containment is complete.

## Neutralize Exposure
- Restart in monitored mode if needed and execute:
  - `sync`
  - `cancel <ORDER_ID>` for outstanding risky orders
  - `flatten` to exit open positions
- Verify account and order state after each action.

## Diagnose Cause
- Inspect recent events for:
  - repeated `agent_intent_executed`
  - `agent_cycle_error`
  - `agent_intent_rejected`
  - fast repeated order submissions
- Check runtime parameters:
  - `-agent-interval`
  - `-agent-max-intents`
  - `-agent-qty`
  - `-agent-move-pct`
  - `-allow`
  - risk limits (`-max-trade`, `-max-day`)

## Controlled Recovery
- Resume in `assist` mode first.
- Use `auto` with `-dry-run` next.
- Re-enable full `auto` only after stable cycles and expected event patterns.

## Post-Incident Hardening
- Reduce per-cycle blast radius:
  - lower `-agent-max-intents`
  - lower `-agent-qty`
  - increase `-agent-interval`
- tighten allowlist and notional limits
- document incident steps in repository notes if workflow changed

## Validation
- Run:
  - `go test ./...`
  - `go build ./cmd/helix`
