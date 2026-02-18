# Project Agent Instructions (`helix-tui`)

## Purpose

Provide consistent operating rules for agents working in this repository, with a strong bias toward safe paper-trading workflows and auditable autonomous execution.

## Non-Negotiables

- Keep risk controls enabled at all times.
- Default to paper mode unless the user explicitly requests otherwise.
- Prefer `-dry-run` before enabling real autonomous execution.
- Never bypass symbol allowlists or notional limits in code changes.
- Treat all autonomous activity as auditable: preserve clear event logging.

## Skills

Use these local skills when relevant:

- `autonomous-trading-ops`
  - File: `skills/autonomous-trading-ops/SKILL.md`
  - Use for running the app in `manual|assist|auto`, choosing flags, and monitoring autonomous behavior.

- `alpaca-paper-runbook`
  - File: `skills/alpaca-paper-runbook/SKILL.md`
  - Use for Alpaca paper setup, credential wiring, paper endpoint operations, and adapter troubleshooting.

- `risk-incident-response`
  - File: `skills/risk-incident-response/SKILL.md`
  - Use for kill-switch style response, flatten/cancel workflows, and safe recovery after abnormal behavior.

## Defaults

- Preferred broker for autonomous testing: `paper`
- Preferred startup for autonomous verification:
  - `-mode=auto -dry-run` first
  - then remove `-dry-run` after behavior is acceptable
- Preferred monitoring:
  - run with TUI (no `-headless`) when operator visibility is needed
  - use `-headless` for daemon-style runs

## Change Expectations

- Keep changes small and reversible.
- Update `README.md` when CLI flags or runtime behavior changes.
- Validate with:
  - `go test ./...`
  - `go build ./cmd/helix`
