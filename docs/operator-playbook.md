# Operator Playbook

Operational guide for running `helix-tui` safely day-to-day.

## Safe Defaults

- Use `[alpaca].env = "paper"` unless explicitly operating live.
- Prefer `mode = "assist"` or `mode = "auto"` with `[agent].dry_run = true` during validation.
- Keep risk and compliance controls enabled.

## Session Start Checklist

1. Confirm config mode/env:
   - `mode`
   - `[alpaca].env`
   - `[agent].dry_run`
2. Start app:
   - `go run ./cmd/helix -config=config.toml`
3. Verify health in TUI:
   - header account values populated
   - Logs tab has no startup auth/sync failures
   - System tab request/persistence counters look healthy

## Core Monitoring Surfaces

- `Overview` tab:
  - watchlist quote state
  - open orders
  - position/equity behavior
- `Logs` tab:
  - `agent_cycle_error`
  - `agent_intent_rejected`
  - `order_placed`
  - `trade_update`
  - `compliance_posture`
  - `compliance_drift_detected`
- `Strategy` tab:
  - active plan summary
  - recommendations
  - active steering context (version/hash/objective/symbol sets)
  - stale/health indicators
- `Chat` tab:
  - copilot thread history
  - operator/assistant strategy discussion context
- `System` tab:
  - request success/failure counts
  - persistence health
  - compliance posture/drift status

## Operational Commands

- `sync`
- `watch list|add|remove|sync`
- `cancel <ORDER_ID|ORDER_ID_PREFIX|#ROW>`
- `flatten`
- `strategy run|status|approve|reject|archive`
- `strategy chat status|list|new|use|say`
- `tab overview|strategy|chat|system|logs`

## Incident Response

1. Contain:
   - stop autonomous run (`q` / terminate process)
2. Neutralize:
   - restart in `manual` or `assist`
   - `sync`
   - `cancel ...`
   - `flatten` if needed
3. Diagnose:
   - inspect rejection reasons and cycle errors
   - verify risk/compliance settings
4. Recover gradually:
   - `assist` -> `auto + dry_run` -> full `auto`

## Live-Mode Caution

Before `[alpaca].env = "live"`:

- confirm strategy/risk/compliance settings
- lower notional and intent limits
- run extended paper validation
- ensure incident procedures are ready
- ensure `compliance_posture` reflects expected account type/PDT counters
- resolve any active `compliance_drift_detected` signal before live cutover

## Useful Files

- config: `config.toml`
- logs: `logs/helix.log`
- db: `data/helix.db`
