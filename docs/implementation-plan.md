# helix-tui Implementation Plan

Last updated: 2026-02-19

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
| _(none)_ |  |  |  |  |

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
