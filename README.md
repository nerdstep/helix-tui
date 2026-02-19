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

- Go 1.22+

## Run

```bash
go mod tidy
go run ./cmd/helix
```

## Config File (TOML)

The app can load runtime settings from a TOML file using `github.com/pelletier/go-toml/v2`.

- Default path: `config.toml` in the project root (auto-loaded if present)
- Override path: `-config=path/to/file.toml`
- Example template: `config.example.toml`
- Alpaca routing config keys:
  - `[alpaca].env`
  - `[alpaca].base_url`
- Agent selection/config keys:
  - `[agent].type` (`heuristic` or `llm`)
  - `[agent].sync_timeout`
  - `[agent].order_timeout`
  - `[agent.llm].api_key`
  - `[agent.llm].base_url`
  - `[agent.llm].model`
  - `[agent.llm].timeout`
  - `[agent.llm].system_prompt`
  - `[logging].file` (optional event log path)
  - `[logging].mode` (`append` or `truncate`)

Config precedence is:

- built-in defaults
- TOML file
- environment variables (`APCA_API_KEY_ID`, `APCA_API_SECRET_KEY`, `APCA_API_DATA_URL`, `OPENAI_API_KEY`, `HELIX_LLM_API_KEY`, `HELIX_LOG_FILE`, `HELIX_LOG_MODE`)
- CLI flags

Windows (PowerShell) quick start:

```powershell
Copy-Item config.example.toml config.toml
go run ./cmd/helix -config=config.toml
```

Useful flags:

```bash
go run ./cmd/helix \
  -config=config.toml \
  -broker=paper \
  -max-trade=5000 \
  -max-day=20000 \
  -allow=AAPL,MSFT,TSLA,NVDA \
  -sync-timeout=15s \
  -order-timeout=15s \
  -log-file=logs/helix.log \
  -log-mode=append \
  -mode=manual \
  -agent-type=heuristic
```

Alpaca broker mode (paper or live environment):

```bash
go run ./cmd/helix \
  -config=config.toml \
  -broker=alpaca \
  -alpaca-env=paper \
  -alpaca-feed=iex \
  -alpaca-key=$APCA_API_KEY_ID \
  -alpaca-secret=$APCA_API_SECRET_KEY \
  -mode=assist
```

Alpaca credentials with keyring (recommended):

```bash
go run ./cmd/helix \
  -config=config.toml \
  -broker=alpaca \
  -alpaca-env=paper \
  -alpaca-feed=iex \
  -use-keyring \
  -save-keyring \
  -keyring-service=helix-tui \
  -keyring-user=alpaca
```

Windows (PowerShell) credential setup:

1. Add your Alpaca paper credentials to the current shell:

```powershell
$env:APCA_API_KEY_ID = "YOUR_KEY_ID"
$env:APCA_API_SECRET_KEY = "YOUR_SECRET_KEY"
```

1. Run once with keyring save enabled to store credentials in Windows Credential Manager:

```powershell
go run ./cmd/helix -broker=alpaca -alpaca-env=paper -alpaca-feed=iex -use-keyring -save-keyring -mode=assist
```

1. Future runs can omit `-alpaca-key` / `-alpaca-secret` and environment variables:

```powershell
go run ./cmd/helix -broker=alpaca -alpaca-env=paper -alpaca-feed=iex -use-keyring -mode=assist
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
  -broker=paper \
  -mode=auto \
  -headless \
  -watchlist=AAPL,MSFT,TSLA \
  -agent-interval=10s \
  -agent-qty=1 \
  -agent-move-pct=0.01 \
  -agent-max-intents=1
```

LLM autonomous loop (safe startup):

```bash
OPENAI_API_KEY=your_key \
go run ./cmd/helix \
  -config=config.toml \
  -broker=alpaca \
  -alpaca-env=paper \
  -mode=auto \
  -agent-type=llm \
  -dry-run \
  -llm-model=gpt-4.1-mini \
  -agent-max-intents=1
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

`-dry-run` works with autonomous modes and logs intents without submitting orders.

Agent implementations:

- `heuristic`: built-in deterministic price-move strategy.
- `llm`: LLM proposes trade intents from snapshot/watchlist/quotes/events context (implemented with official `github.com/openai/openai-go`).

## Safety Defaults

- Symbol allowlist
- Max notional per trade
- Max daily notional

These checks are enforced in `internal/engine/risk.go`.

## Notes

- The paper broker fills immediately at the current mock quote.
- The Alpaca adapter uses the official SDK client for account/positions/orders plus market data latest quote and trade update streaming.
- Alpaca market data permissions/entitlements still apply to quote availability.
- Use `-broker=alpaca -alpaca-env=paper|live` to choose trading environment; optional `-alpaca-base-url` overrides endpoint routing.
- Alpaca quote feed defaults to `iex` (override via `-alpaca-feed`).
- When `-use-keyring` is enabled, missing Alpaca credentials are loaded from OS keyring; provided credentials can be stored with `-save-keyring`.
- When `-use-keyring` is enabled, missing LLM credentials are also loaded from OS keyring; provided `-llm-key` values can be stored with `-save-keyring`.
- In `-broker=alpaca`, the app treats Alpaca watchlist `helix-tui` as the watchlist source of truth.
- In `-broker=paper`, watchlist comes from config/flags only.
- Watchlist symbols are automatically treated as allowlisted symbols by the risk gate.
- The built-in agent is a conservative heuristic (`internal/agent/heuristic`) that reacts to sampled price moves.
- LLM agent can be enabled with `-agent-type=llm` (or `[agent].type = "llm"`).
- LLM credentials can be supplied via `OPENAI_API_KEY` / `HELIX_LLM_API_KEY`, config, `-llm-key`, or keyring.
- LLM output only proposes intents; all execution still goes through `Runner -> Engine -> RiskGate`.
- Set `-log-file=...` (or `[logging].file`) to persist event logs for later debugging.
- Use `-log-mode=truncate` (or `[logging].mode = "truncate"`) to reset the log file each app start.
- The TUI includes watchlist quote rows, position P&L, and basic agent/system runtime stats.
- Event history supports keyboard paging (`PgUp`, `PgDn`, `Home`, `End`) and retains a larger recent window for scrollback.
