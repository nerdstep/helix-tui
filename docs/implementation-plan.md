# helix-tui Implementation Plan

Last updated: 2026-02-21

This is the running backlog for product/architecture ideas that are approved, in progress, or parked for later.

## Workflow

- Add new ideas to `Backlog`.
- Move selected items to `Next Up`.
- Move active work to `In Progress`.
- Mark items `Done` with completion notes.

Status values:

- `proposed`
- `next`
- `in_progress`
- `blocked`
- `done`
- `parked`

## Next Up

| ID | Item | Priority | Status | Why | Exit Criteria |
|---|---|---|---|---|---|
| ALPACA-001 | Investigate error: {"level":"warn","component":"eventlog","event_type":"agent_intent_rejected","event_time":"2026-02-19T20:29:57-08:00","time":"2026-02-19T20:29:58-08:00","message":"buy RIVN qty=100.00 type=limit conf=0.00 gain=6.00% rationale=No existing position or open orders; last=15.415 — place a small limit buy (100 sh = ~$1.54k, ~1.5% cash) to establish exposure; targeting modest 6% gain.: invalid limit_price 15.415. sub-penny increment does not fulfill minimum pricing criteria (HTTP 422, Code 42210000)"} | high | next |
| AGENT-005 | LLM research tool layer (news/fundamentals input before intent generation) | high | next | Improve decision quality vs quote-only context | Agent prompt includes research payload; tests cover empty/failure research cases; risk-gated execution unchanged |
| AGENT-006 | Assist-mode approval workflow for agent intents in TUI | high | next | Turn `assist` mode into actionable human approval loop | TUI shows pending intents and supports approve/reject commands; events logged for every decision |

## Backlog

| ID | Item | Priority | Status | Why | Exit Criteria |
|---|---|---|---|---|---|
| AGENT-004 | Migrate LLM agent from Chat Completions to Responses API | medium | parked | Better long-term support for tools/stateful agent flows | LLM adapter uses `client.Responses.New`; intent parsing + tests preserved; behavior parity with current agent |
| AGENT-007 | Prompt/version registry for agent prompts | medium | proposed | Controlled prompt evolution and rollback | Prompt ID/version in config; current prompt file- and version-addressable; events include prompt version |
| OPS-003 | Persistent run ledger (cycles/intents/orders/rejections) | medium | proposed | Auditable autonomous behavior across sessions | File-backed ledger with rotation; includes cycle summary + intent decisions + execution outcomes |
| SAFETY-003 | Live-trading enablement guardrail | high | proposed | Prevent accidental live deployment | Explicit `live_enable=true` gate + startup confirmation event when `alpaca.env=live` |
| DX-002 | Backtest/replay harness for agent strategies | medium | proposed | Faster strategy iteration without live loops | Deterministic replay command over stored snapshots/quotes; performance and intent stats output |

## In Progress

| ID | Item | Priority | Status | Notes |
|---|---|---|---|---|
| COMPLIANCE-001 | Live-trading compliance guardrails (PDT/GFV) | high | in_progress | Phase 1 + Phase 2 implemented; Alpaca calendar now used as settlement source in broker=alpaca mode |

## Compliance Rollout Plan (Phased)

### Phase 1: PDT-aware pre-trade guard (implemented)

- Status: `done`
- Scope:
  - Add `ComplianceGate` invoked by `Engine.PlaceOrder` after `RiskGate`.
  - Add config surface under `[compliance]`:
    - `enabled`
    - `account_type`
    - `avoid_pdt`
    - `max_day_trades_5d`
    - `min_equity_for_pdt`
    - `avoid_gfv` (reserved for later phase)
  - Emit `compliance_rejected` events on blocked orders.
  - Surface compliance rejection counts in TUI System tab.
- Completion criteria:
  - [x] Manual and autonomous orders both pass through compliance checks.
  - [x] Config parsing/normalization/validation covers compliance fields.
  - [x] Tests cover PDT-block and allow paths.
  - [x] README and architecture docs updated.

### Phase 2: Cash-account settlement/GFV guard (implemented)

- Status: `done`
- Scope:
  - Build settled/unsettled cash ledger from fills + settlement rules (T+1 baseline).
  - Use Alpaca market calendar as settlement source of truth when `broker=alpaca`.
  - Block buy orders that would consume unsettled proceeds in a way likely to trigger GFV/freeriding restrictions.
  - Emit explicit rejection reasons (`gfv_guard`) and add TUI counters.
- Completion criteria:
  - [x] Persisted fill/settlement state model in SQLite.
  - [x] Deterministic pre-trade checks with unit tests for common cash-account flows.
  - [x] Configurable strictness and account-type overrides.

### Phase 3: Broker-aware compliance reconciliation (planned)

- Status: `proposed`
- Scope:
  - Reconcile local compliance state with broker/account flags each sync cycle.
  - Include broker-reported PDT indicators in system status and decision context.
- Completion criteria:
  - [ ] Startup/system events include broker compliance posture.
  - [ ] Drift detection between local estimates and broker-reported values.
  - [ ] Operator runbook updates for live cutover and incident response.

## Done

| ID | Item | Completed | Notes |
|---|---|---|---|
| AGENT-001 | Pluggable agent type (`heuristic` / `llm`) | 2026-02-19 | Config + CLI wired; runner uses selected agent |
| AGENT-002 | LLM agent implementation with strict JSON intent parsing | 2026-02-19 | Intents filtered by watchlist; still executed only through risk-gated engine |
| AGENT-003 | Official OpenAI Go SDK integration (`openai-go`) | 2026-02-19 | Replaced raw HTTP client in LLM adapter |

## Item Template

Use this when adding a new item:

```md
| AREA-### | Short title | high/medium/low | proposed | one-line reason | concrete, testable completion criteria |
```
