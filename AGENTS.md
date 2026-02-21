# Project Agent Instructions (`helix-tui`)

## Purpose

Provide consistent rules for coding/ops agents working in this repository, with a strong safety bias and auditable autonomous behavior.

## Non-Negotiables

- Keep risk controls enabled.
- Default to Alpaca paper environment (`[alpaca].env = "paper"`) unless explicitly asked to use live.
- Prefer `mode = "auto"` with `[agent].dry_run = true` before enabling real autonomous execution.
- Never bypass watchlist allowlist behavior or notional limits.
- Preserve auditable events/logging for autonomous actions.

## Local Skills

- `skills/autonomous-trading-ops/SKILL.md`
- `skills/alpaca-paper-runbook/SKILL.md`
- `skills/risk-incident-response/SKILL.md`

Use these when task scope matches runtime operations, Alpaca wiring, or safety incidents.

## Defaults

- Runtime broker model: Alpaca (paper/live via config)
- Preferred safe startup for autonomous validation:
  - `[alpaca].env = "paper"`
  - `mode = "auto"`
  - `[agent].dry_run = true`
- Preferred visibility:
  - use TUI (no `-headless`) when operator monitoring is needed
  - use `-headless` for unattended runs

## Change Expectations

- Keep changes small and reversible.
- Update `README.md` and relevant docs when runtime behavior or operations change.
- Validate with:
  - `go test ./...`
  - `go build ./cmd/helix`
