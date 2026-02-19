# helix-tui

Go CLI + TUI trading app scaffold with a safety-first architecture:

- Broker adapter boundary (`paper` simulator and `alpaca` real API mode)
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
  - In-memory paper broker with instant fills
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
- Alpaca routing config keys:
  - `[alpaca].env`
  - `[alpaca].base_url`
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
  - `[agent.llm].system_prompt`
  - `[logging].file` (optional event log path)
  - `[logging].mode` (`append` or `truncate`)
  - `[logging].level` (`trace|debug|info|warn|error|fatal|panic|disabled`)
  - `[database].path` (SQLite database path for persistent state)

Config precedence is:

- built-in defaults
- TOML file
- environment variables for credentials (`APCA_API_KEY_ID`, `APCA_API_SECRET_KEY`, `APCA_API_DATA_URL`, `OPENAI_API_KEY`, `HELIX_LLM_API_KEY`)
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
- `cancel <ORDER_ID>`
- `flatten`
- `sync`
- `events up|down|top|tail [N]` (scroll event history)
- `watch list`
- `watch add <SYM>`
- `watch remove <SYM>`
- `watch sync` (or `watch pull`)
- `help`
- `q`

## Runtime Modes

- `manual`: agent intents are ignored; only manual commands trade.
- `assist`: agent generates intents but does not execute, and logs `agent_intent_needs_approval` events.
- `auto`: agent intents are executed automatically through the same risk gate as manual orders.

`[agent].dry_run` works with autonomous modes and logs intents without submitting orders.
`[agent].min_gain_pct` enforces a minimum expected gain percent per intent (0 disables).

Agent implementations:

- `heuristic`: built-in deterministic price-move strategy.
- `llm`: LLM proposes trade intents from snapshot/watchlist/quotes/events context (implemented with official `github.com/openai/openai-go`).

Agent tuning notes:

- `[agent].qty`: heuristic agent only; fixed quantity used when heuristic emits intents.
- `[agent].move_pct`: heuristic agent only; absolute sampled price-move threshold (`0.01` = `1%`) before signaling.
- LLM-specific TOML settings live under `[agent.llm]`.
- `[agent.llm].system_prompt`: primary instruction channel for LLM behavior and goals.

## Safety Defaults

- Symbol allowlist
- Max notional per trade
- Max daily notional

These checks are enforced in `internal/engine/risk.go`.

## Notes

- The paper broker fills immediately at the current mock quote.
- The Alpaca adapter uses the official SDK client for account/positions/orders plus market data latest quote and trade update streaming.
- Alpaca market data permissions/entitlements still apply to quote availability.
- Use `[alpaca].env = "paper|live"` to choose trading environment; optional `[alpaca].base_url` overrides endpoint routing.
- Alpaca quote feed defaults to `iex` (override via `[alpaca].feed`).
- When `[keyring].use` is enabled, missing Alpaca credentials are loaded from OS keyring; provided credentials can be stored when `[keyring].save` is true.
- When `[keyring].use` is enabled, missing LLM credentials are also loaded from OS keyring.
- In `broker = "alpaca"`, the app treats Alpaca watchlist `helix-tui` as the watchlist source of truth.
- In `broker = "paper"`, watchlist comes from config.
- Watchlist symbols are automatically treated as allowlisted symbols by the risk gate.
- The built-in agent is a conservative heuristic (`internal/agent/heuristic`) that reacts to sampled price moves.
- LLM agent can be enabled with `[agent].type = "llm"`.
- LLM credentials can be supplied via `OPENAI_API_KEY` / `HELIX_LLM_API_KEY`, config, or keyring.
- LLM output only proposes intents; all execution still goes through `Runner -> Engine -> RiskGate`.
- Set `[logging].file` to persist event logs for later debugging.
- Use `[logging].mode = "truncate"` to reset the log file each app start.
- Set `[logging].level` to tune verbosity; default is `info`.
- Event logs are emitted as structured JSON lines (`zerolog`) for easier filtering/analysis.
- High-frequency loop events (`sync`, `agent_cycle_start`, `agent_proposal`, `agent_cycle_complete`, `agent_heartbeat`) are logged at `debug`.
- Set `[database].path` to persist app state in SQLite; equity history is stored there and rendered as a session-spanning P/L trend chart in the TUI.
- SQLite persistence runs startup migrations from `internal/storage` and tracks applied versions in `schema_migrations`.
- The TUI includes watchlist quote rows, position P&L, and basic agent/system runtime stats.
- Equity trend rendering uses `github.com/NimbleMarkets/ntcharts` (sparkline) for higher fidelity terminal charts.
- Event history supports keyboard paging (`PgUp`, `PgDn`, `Home`, `End`) and retains a larger recent window for scrollback.
- Autonomous mode emits periodic `agent_heartbeat` summary events so idle-but-healthy loops are visible in logs.
