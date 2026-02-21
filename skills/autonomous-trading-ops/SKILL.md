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
go run ./cmd/helix -dry-run
```

## Choose Runtime Shape
- Use TUI mode (omit `-headless`) when human monitoring is required.
- Use `-headless` for unattended execution with periodic console summaries.

Example headless run:

```bash
go run ./cmd/helix -headless
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
- Use strategy controls for handoff:
  - `strategy status`
  - `strategy approve <PLAN_ID>` to promote a plan active
  - `strategy reject <PLAN_ID>` to supersede a plan
  - `strategy archive <PLAN_ID>` to remove stale plans from active consideration
- Use `q` to quit TUI.

## Strategy Handoff + Rollback
- Handoff flow:
  - Run `strategy run` to generate/update recommendations.
  - Validate in Strategy tab (`summary`, `recommendations`, `health`).
  - Promote with `strategy approve <PLAN_ID>`.
- Incident rollback flow:
  - `strategy reject <PLAN_ID>` to supersede a bad plan quickly.
  - If positions/orders are unstable, execute `cancel` and `flatten`.
  - Optionally `strategy archive <PLAN_ID>` after incident review.

## Validation
- Run:
  - `go test ./...`
  - `go build ./cmd/helix`
