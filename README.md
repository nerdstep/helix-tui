# helix-tui

Go CLI + TUI trading app scaffold with a safety-first architecture:

- Broker adapter boundary (runtime uses Alpaca; `paper` adapter retained for tests)
- Deterministic risk gate between intent and execution
- State engine with event log and reconciliation
- Autonomous agent runtime modes (`manual`, `assist`, `auto`)
- Terminal UI command cockpit (Bubble Tea)

This repo is a foundation for the architecture discussed in your shared thread, not a production bot.

## Architecture

Diagrams and data flow docs: `docs/architecture.md`
Implementation backlog and roadmap: `docs/implementation-plan.md`

- `cmd/helix`
  - Application entrypoint and CLI flags
- `internal/domain`
  - Core types and interfaces (`Broker`, `Agent`, orders, positions, snapshots)
- `internal/engine`
  - Trading engine + risk gate
- `internal/broker/paper`
  - In-memory paper broker retained for deterministic tests
- `internal/broker/alpaca`
  - Official Alpaca Go SDK adapter (`alpaca-trade-api-go/v3`)
- `internal/tui`
  - Bubble Tea terminal UI

## Prerequisites

- Go 1.24+

## Run

```bash
go mod tidy
go run ./cmd/helix
```

## Config File (TOML)

The app can load runtime settings from a TOML file.

- Default path: `config.toml` in the project root (auto-loaded if present)
- Override path: `-config=path/to/file.toml`
- Example template: `config.example.toml`
- Runtime broker:
  - Runtime always uses Alpaca.
  - The in-memory `paper` adapter is retained for tests only.
- Alpaca routing config keys:
  - `[alpaca].env`
  - `[alpaca].base_url`
- Compliance config keys:
  - `[compliance].enabled`
  - `[compliance].account_type` (`auto|margin|cash`)
  - `[compliance].avoid_pdt`
  - `[compliance].max_day_trades_5d`
  - `[compliance].min_equity_for_pdt`
  - `[compliance].avoid_gfv`
  - `[compliance].settlement_days` (T+N business days for cash-settlement guard)
- Agent selection/config keys:
  - `[agent].type` (`heuristic` or `llm`)
  - `[agent].qty` (heuristic agent only)
  - `[agent].move_pct` (heuristic agent only)
  - `[agent].sync_timeout`
  - `[agent].order_timeout`
  - `[agent].min_gain_pct`
  - `[agent.llm].api_key`
  - `[agent.llm].base_url`
  - `[agent.llm].model`
  - `[agent.llm].timeout`
  - `[agent.llm].context_log` (`off|summary|full`)
  - `[agent.llm].system_prompt`
  - `[strategy].enabled` (low-frequency strategy analyst overseer)
  - `[strategy].interval`
  - `[strategy].auto_activate`
  - `[strategy].max_recommendations`
  - `[strategy].objective`
  - `[strategy.llm].model`
  - `[strategy.llm].timeout`
  - `[strategy.llm].prompt_version`
  - `[strategy.llm].system_prompt`
  - `[logging].file` (optional event log path)
  - `[logging].mode` (`append` or `truncate`)
  - `[logging].level` (`trace|debug|info|warn|error|fatal|panic|disabled`)
  - `[database].path` (SQLite database path for persistent state)

Config precedence is:

- built-in defaults
- TOML file
- environment variables for credentials (`APCA_API_KEY_ID`, `APCA_API_SECRET_KEY`, `APCA_API_DATA_URL`, `OPENAI_API_KEY`)
- CLI flags (`-config` and `-headless`)

Windows (PowerShell) quick start:

```powershell
Copy-Item config.example.toml config.toml
go run ./cmd/helix -config=config.toml
```

Runtime flags (minimal):

```bash
go run ./cmd/helix \
  -config=config.toml \
  -headless
```

Windows (PowerShell) credential setup:

1. Add your Alpaca paper credentials to the current shell:

```powershell
$env:APCA_API_KEY_ID = "YOUR_KEY_ID"
$env:APCA_API_SECRET_KEY = "YOUR_SECRET_KEY"
```

1. Ensure keyring settings in `config.toml` are enabled:

```powershell
go run ./cmd/helix -config=config.toml
```

1. Future runs can omit explicit credential env vars:

```powershell
go run ./cmd/helix -config=config.toml
```

Optional: persist environment variables for new terminals (if you still want env-based auth):

```powershell
setx APCA_API_KEY_ID "YOUR_KEY_ID"
setx APCA_API_SECRET_KEY "YOUR_SECRET_KEY"
```

Open a new terminal after `setx` for changes to take effect.

Headless autonomous loop:

```bash
go run ./cmd/helix \
  -config=config.toml \
  -headless
```

LLM autonomous loop (safe startup):

```bash
OPENAI_API_KEY=your_key \
go run ./cmd/helix \
  -config=config.toml \
  -headless
```

## TUI Commands

- `buy <SYM> <QTY>`
- `sell <SYM> <QTY>`
- `cancel <ORDER_ID|ORDER_ID_PREFIX|#ROW>`
- `flatten`
- `sync`
- `events up|down|top|tail [N]` (scroll event history)
- `watch list`
- `watch add <SYM>`
- `watch remove <SYM>`
- `watch sync` (or `watch pull`)
- `strategy run` (queue immediate strategy analyst cycle)
- `strategy status`
- `help`
- `q`

## Runtime Modes

- `manual`: agent intents are ignored; only manual commands trade.
- `assist`: agent generates intents but does not execute, and logs `agent_intent_needs_approval` events.
- `auto`: agent intents are executed automatically through the same risk gate as manual orders.

`[agent].dry_run` works with autonomous modes and logs intents without submitting orders.
`[agent].min_gain_pct` enforces a minimum expected gain percent per intent (0 disables).
Autonomous cycles run on the configured interval, but agent invocation is skipped when the decision context is unchanged (with periodic forced refresh).

Agent implementations:

- `heuristic`: built-in deterministic price-move strategy.
- `llm`: LLM proposes trade intents from snapshot/watchlist/quotes/events context (implemented with official `github.com/openai/openai-go`).

Agent tuning notes:

- `[agent].qty`: heuristic agent only; fixed quantity used when heuristic emits intents.
- `[agent].move_pct`: heuristic agent only; absolute sampled price-move threshold (`0.01` = `1%`) before signaling.
- LLM-specific TOML settings live under `[agent.llm]`.
- `[agent.llm].system_prompt`: primary instruction channel for LLM behavior and goals.
- `[agent.llm].context_log`: request-context logging for LLM mode (`summary` recommended for debugging, `full` emits full JSON payload events).
- LLM request context includes risk limits from `[risk]` (`max_trade_notional`, `max_day_notional`) so the model can size intents within policy.
- Rejected intents are included in `recent_events` with a dedicated `rejection_reason` field (separate from event `details`).
- System tab now surfaces agent request counters (`ok`/`failed`) and DB event persistence health (`queue`, `flush_ok`, `flush_failed`, `events_ok`, `events_failed`, `dropped`).
- Compliance guard can reject orders with `compliance_rejected` when PDT/GFV protection blocks risky buys.

## Safety Defaults

- Watchlist-based symbol allowlist
- Max notional per trade
- Max daily notional
- Optional compliance guard for live scenarios (PDT + cash-account GFV guardrails)

These checks are enforced in `internal/engine/risk.go` and `internal/engine/compliance.go`.

## Notes

- The Alpaca adapter uses the official SDK client for account/positions/orders plus trade update streaming and websocket quote streaming.
- In Alpaca mode, quotes are streamed independently from the agent interval into an engine quote cache.
- Runner/TUI quote reads use the cached stream first and fall back to REST latest-quote lookup when cache is stale or missing.
- Alpaca market data permissions/entitlements still apply to quote availability.
- Use `[alpaca].env = "paper|live"` to choose trading environment; optional `[alpaca].base_url` overrides endpoint routing.
- Alpaca quote feed defaults to `iex` (override via `[alpaca].feed`).
- When `[keyring].use` is enabled, missing Alpaca credentials are loaded from OS keyring; provided credentials can be stored when `[keyring].save` is true.
- When `[keyring].use` is enabled, missing LLM credentials are also loaded from OS keyring.
- Runtime treats Alpaca watchlist `helix-tui` as the watchlist source of truth.
- Watchlist symbols are automatically treated as allowlisted symbols by the risk gate.
- The built-in agent is a conservative heuristic (`internal/agent/heuristic`) that reacts to sampled price moves.
- LLM agent can be enabled with `[agent].type = "llm"`.
- LLM credentials can be supplied via `OPENAI_API_KEY`, config, or keyring.
- LLM output only proposes intents; all execution still goes through `Runner -> Engine -> RiskGate`.
- LLM/manual order execution still passes through pre-trade controls; when enabled, `ComplianceGate` runs after `RiskGate`.
- `avoid_gfv` uses a SQLite-backed unsettled-proceeds ledger built from observed sell fills (cash accounts), with settlement based on `[compliance].settlement_days`.
- Settlement-day resolution for GFV guardrails uses Alpaca calendar API data as source of truth.
- Set `[logging].file` to persist event logs for later debugging.
- Use `[logging].mode = "truncate"` to reset the log file each app start.
- Set `[logging].level` to tune verbosity; default is `info`.
- Event logs are emitted as structured JSON lines (`zerolog`) for easier filtering/analysis.
- High-frequency loop events (`sync`, `agent_cycle_start`, `agent_proposal`, `agent_cycle_complete`, `agent_heartbeat`) are logged at `debug`.
- Set `[database].path` to persist app state in SQLite; equity history, relevant trade/agent execution events, and strategy plan memory are stored there.
- In LLM mode, recent event context is sourced from persisted DB events (not only in-memory session events), so context survives restarts.
- Relevant trade/agent events are persisted at event-emission time (engine -> runtime persistor -> SQLite) in transactional batches.
- SQLite persistence auto-applies the current schema at startup from `internal/storage`.
- The TUI includes watchlist quote rows, position P&L, and basic agent/system runtime stats.
- The TUI now includes a dedicated `Strategy` tab for active plan, recommendations, recent plan history, and strategy health/staleness status.
- Strategy runner skips scheduled/startup analysis when the latest stored plan is still within `[strategy].interval`; use `strategy run` to force an immediate cycle.
- Strategy analyst supports a no-change outcome (`strategy_plan_unchanged`) to retain the current plan instead of creating a new plan record.
- Equity trend rendering uses `github.com/NimbleMarkets/ntcharts` (sparkline) for higher fidelity terminal charts.
- Event history supports keyboard paging (`PgUp`, `PgDn`, `Home`, `End`) and retains a larger recent window for scrollback.
- Autonomous mode emits periodic `agent_heartbeat` summary events so idle-but-healthy loops are visible in logs.
