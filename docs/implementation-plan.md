# helix-tui Implementation Plan

Last updated: 2026-02-25

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

## Loop-Closure Objective (Primary)

Target state for "closing the loop":

- strategy outputs become enforceable execution constraints (not advisory-only metadata)
- autonomous intents are either executed or explicitly rejected with machine-readable reasons
- every cycle is attributable end-to-end: plan -> intent -> order -> fill -> realized/unrealized outcome
- outcome metrics feed back into future strategy generation and operator steering decisions

Definition of done for loop closure:

- no known critical execution blockers in paper auto runs (timeouts/tick-size rejects)
- policy enforcement covers more than symbol/bias/max-notional
- persistent run ledger + outcome attribution is queryable in UI and DB
- analyst/copilot prompts include recent measured outcomes, not only snapshots/events

## Next Up

| ID | Item | Priority | Status | Why | Exit Criteria |
|---|---|---|---|---|---|
| ALPACA-001 | Execution reliability hardening (timeouts + invalid limit increments) | high | next | Autonomous loop cannot close reliably when sync/order edge cases break cycles | Resolve `get open orders: context deadline exceeded`; enforce/round valid tick increments pre-submit; add tests + explicit events for normalization |
| AGENT-006 | Assist-mode approval workflow for agent intents in TUI | high | next | Turn `assist` mode into actionable human approval loop | TUI shows pending intents and supports approve/reject commands; events logged for every decision |
| POLICY-001 | Expand strategy policy enforcement depth | high | next | Current execution policy bridge is shallow; plans are not fully actionable constraints | Enforce entry bands, stale-plan gating, and recommendation-level execution constraints in autonomy path |
| OPS-003 | Persistent run ledger (cycles/intents/orders/fills/rejections/outcomes) | high | next | Loop closure needs traceable attribution, not just sparse events | DB-backed ledger links cycle->intent->order->trade outcome with query APIs and TUI summaries |

## Backlog

| ID | Item | Priority | Status | Why | Exit Criteria |
|---|---|---|---|---|---|
| METRICS-001 | Portfolio/trade outcome metrics surface | high | proposed | Compounding requires measurable performance and risk, not just execution events | Persist + display rolling return, drawdown, hit-rate, avg win/loss, exposure, and turnover |
| FEEDBACK-001 | Outcome-aware analyst context injection | high | proposed | Strategy quality should improve from realized outcomes | Strategy input includes trailing outcome block (per-symbol and portfolio); plan events include metric window used |
| RISK-004 | Portfolio-level risk circuit breakers | high | proposed | Per-trade limits alone are insufficient for autonomous compounding safety | Configurable daily loss, drawdown, and exposure breakers that halt auto execution with explicit events |
| SAFETY-003 | Live-trading enablement guardrail | high | proposed | Prevent accidental live deployment | Explicit `live_enable=true` gate + startup confirmation event when `alpaca.env=live` |
| EVAL-001 | Paper replay/backtest harness for strategy variants | medium | proposed | Need objective comparison before promoting strategy changes | Deterministic replay mode computes strategy KPIs and compares against baseline configurations |

## In Progress

| ID | Item | Priority | Status | Notes |
|---|---|---|---|---|
| COMPLIANCE-001 | Live-trading compliance guardrails (PDT/GFV) | high | in_progress | Phase 1-3 implemented; remaining work focuses on event noise tuning and live cutover integration |

## Closing The Loop Rollout Plan (Phased)

### Phase 1: Reliable autonomous actuation (target now)

- Status: `next`
- Scope:
  - remove known autonomous blockers in paper mode (sync/open order timeout and invalid limit increments)
  - complete assist-mode approval workflow so intents can be actioned safely by operator
  - finish steering apply/reject lifecycle to convert chat outcomes into approved strategy input
- Completion criteria:
  - [ ] `ALPACA-001` completed with regression coverage
  - [ ] `AGENT-006` completed with auditable approval/rejection events
  - [x] `STRAT-009` completion criteria fully met

### Phase 2: Enforceable strategy execution policy

- Status: `proposed`
- Scope:
  - expand runtime enforcement from symbol/bias/max-notional to recommendation semantics
  - add stale-plan and plan-health gating for auto mode
  - ensure rejected intents return explicit policy reason codes for attribution
- Completion criteria:
  - [ ] `POLICY-001` completed
  - [ ] policy rejection reasons are structured and persisted for analysis
  - [ ] strategy policy behavior is covered by autonomy/adapter tests

### Phase 3: Attribution and performance measurement

- Status: `proposed`
- Scope:
  - implement persistent run ledger and outcome attribution joins
  - surface actionable KPIs in System/Strategy views
  - add exports for offline analysis and incident review
- Completion criteria:
  - [ ] `OPS-003` completed
  - [ ] `METRICS-001` completed
  - [ ] operators can trace plan->intent->order->outcome for any recent execution

### Phase 4: Adaptive feedback and continuous improvement

- Status: `proposed`
- Scope:
  - inject measured outcomes into analyst cycle input
  - support periodic strategy retrospectives based on hard metrics
  - add evaluation harness for variant testing before strategy/prompt changes are promoted
- Completion criteria:
  - [ ] `FEEDBACK-001` completed
  - [ ] `EVAL-001` completed
  - [ ] plan-generation events record outcome window + metric snapshot used

## Strategy Analyst Rollout Plan (Phased)

### Phase 1: Strategy memory foundation (implemented)

- Status: `done`
- Scope:
  - Add typed persistence for strategy plans/recommendations in SQLite.
  - Add repository APIs for create/list/activate/get-active plan workflows.
  - Ensure plan activation supersedes prior active plans for single-source strategy state.
- Completion criteria:
  - [x] New DB tables auto-migrated at startup.
  - [x] Repository tests cover create/read/activate/supersede and reopen persistence.
  - [x] Store exposes strategy repository for runtime/agent wiring.

### Phase 2: Analyst agent runtime loop (implemented)

- Status: `done`
- Scope:
  - Add a low-frequency `strategy` runner (overseer cadence).
  - Add analyst prompt contract for structured plan + picks output.
  - Persist model metadata and resulting plans/recommendations.
  - Surface strategy state in a dedicated TUI `Strategy` tab (not System tab).
- Completion criteria:
  - [x] Analyst loop runs independently from execution loop.
  - [x] Dedicated Strategy tab shows active plan + recommendations + recent plan history.
  - [x] Unit tests cover strategy runner persistence/activation path.
  - [x] Add stale-plan indicators + explicit last-run health status.

### Phase 3: Execution integration + operator controls (implemented)

- Status: `done`
- Scope:
  - Execution agent consumes only active strategy constraints.
  - Add approve/reject/archive controls for strategy plans (initially via TUI commands).
  - Surface active strategy summary in TUI/System.
- Completion criteria:
  - [x] Execution intents are policy-checked against active strategy constraints.
  - [x] Operator can promote/supersede/archive plans with clear audit events.
  - [x] Docs/runbooks cover strategy handoff and incident rollback.

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

### Phase 3: Broker-aware compliance reconciliation (implemented)

- Status: `done`
- Scope:
  - Reconcile local compliance state with broker/account flags each sync cycle.
  - Include broker-reported PDT indicators in system status and decision context.
- Completion criteria:
  - [x] Startup/system events include broker compliance posture.
  - [x] Drift detection between local estimates and broker-reported values.
  - [x] Operator runbook updates for live cutover and incident response.

## Strategy Copilot Rollout Plan (Phased)

### Phase 0: LLM platform precursor (implemented)

- Status: `done`
- Scope:
  - Migrate execution LLM adapter from Chat Completions to Responses API.
  - Preserve strict JSON output contract and retry behavior.
- Completion criteria:
  - [x] `internal/agent/llm` uses `client.Responses.New`.
  - [x] Existing parsing/guard tests pass with no behavior regressions.

### Phase 1: Advisory chat memory + operator UX

- Status: `done`
- Scope:
  - Add persistent strategy conversation threads/messages in SQLite.
  - Add `strategy chat` command surface and dedicated Chat tab viewport/input.
  - Keep chat advisory-only (no direct order execution side effects).
- Completion criteria:
  - [x] Operator can create/select/list chat threads.
  - [x] Messages persist across app restarts and are queryable in UI.
  - [x] Chat actions emit auditable events.

### Phase 2: Copilot outcomes -> Analyst steering contract

- Status: `done`
- Scope:
  - Persist structured steering state derived from chat decisions:
    - preferred symbols/themes
    - risk posture and sizing preferences
    - constraints and exclusions
    - planning horizon / objective emphasis
  - Introduce explicit operator apply/reject commands for proposed steering updates.
  - Inject active steering state into analyst input every strategy cycle.
  - Surface active steering state and last update source in Strategy tab.
- Completion criteria:
  - [x] Steering state is stored in typed DB tables (not free-form blobs).
  - [x] Analyst prompt payload includes steering state deterministically.
  - [x] Plan generation events record steering version/hash used.
  - [x] Operator can inspect/approve/reject steering updates with audit events.

### Phase 3: Structured proposals + approval workflow

- Status: `done`
- Scope:
  - Chat agent returns structured proposal blocks:
    - `watchlist_proposal` (add/remove symbols)
    - `steering_proposal` (constraints/objective updates)
  - Add explicit apply commands with audit trail.
- Completion criteria:
  - [x] Operator can apply/reject proposals explicitly.
  - [x] Applied watchlist changes sync with Alpaca and local execution scope.
  - [x] Proposal decisions are persisted and visible in Strategy tab history.

### Phase 4: Research tooling + richer context

- Status: `proposed`
- Scope:
  - Add optional research tools/connectors (news/fundamental/filings/market breadth).
  - Add citation-aware outputs and confidence scoring.
  - Add configurable guardrails for research freshness and source trust.
- Completion criteria:
  - [ ] Chat responses can include structured citations/sources.
  - [ ] Operator can view source links and rationale in Strategy tab.
  - [ ] Tool failures degrade gracefully without breaking chat loop.

## Done

| ID | Item | Completed | Notes |
|---|---|---|---|
| AGENT-001 | Pluggable agent type (`heuristic` / `llm`) | 2026-02-19 | Config + CLI wired; runner uses selected agent |
| AGENT-002 | LLM agent implementation with strict JSON intent parsing | 2026-02-19 | Intents filtered by watchlist; still executed only through risk-gated engine |
| AGENT-003 | Official OpenAI Go SDK integration (`openai-go`) | 2026-02-19 | Replaced raw HTTP client in LLM adapter |
| AGENT-004 | Migrate LLM agent from Chat Completions to Responses API | 2026-02-23 | `internal/agent/llm` now uses `client.Responses.New` with JSON object output format; retry/parsing behavior preserved |
| AGENT-005 | Strategy Analyst overseer (deep research + plan memory) | 2026-02-21 | Phase 1-3 completed: DB memory, analyst runner + Strategy tab, active-plan execution constraints, and TUI strategy plan controls |
| STRAT-008 | Strategy Copilot chat (advisory + proposals) | 2026-02-25 | Copilot now emits structured proposals; operator can inspect/apply/reject in TUI with audit events |
| STRAT-009 | Copilot-to-Analyst steering contract | 2026-02-25 | Steering apply/reject workflow completed; approved steering is persisted, visible, and consumed by analyst loop |

## Item Template

Use this when adding a new item:

```md
| AREA-### | Short title | high/medium/low | proposed | one-line reason | concrete, testable completion criteria |
```
