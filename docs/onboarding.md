# New User Onboarding

This guide walks a new operator from zero to a safe first session.

## 1) Prerequisites

- Go 1.24+
- Alpaca account and API keys (paper recommended)
- Optional: OpenAI API key for `agent.type = "llm"` and strategy analyst

## 2) Create Your Config

From repository root:

```powershell
Copy-Item config.example.toml config.toml
```

Edit `config.toml` and confirm:

- `mode = "manual"`
- `[alpaca].env = "paper"`
- `[agent].dry_run = true` (if you plan to test `mode = "auto"` soon)

Optional identity fields:

```toml
[identity]
human_name = "Your Name"
human_alias = "@your_handle"
agent_name = "Helix"
```

## 3) Configure Credentials

You can use config, env vars, or keyring. Recommended: keyring-enabled with env vars on first run.

PowerShell example:

```powershell
$env:APCA_API_KEY_ID = "YOUR_PAPER_KEY"
$env:APCA_API_SECRET_KEY = "YOUR_PAPER_SECRET"
```

If using LLM/strategy:

```powershell
$env:OPENAI_API_KEY = "YOUR_OPENAI_KEY"
```

## 4) First Launch (Manual)

```bash
go run ./cmd/helix -config=config.toml
```

Verify in TUI:

- header shows account values
- watchlist loads
- no auth/sync errors in Logs tab
- `sync` command succeeds

## 5) Validate Command Path

In manual mode, run small tests:

- `watch list`
- `watch add AAPL` / `watch remove AAPL`
- `sync`

If testing order flow, use very small quantities and confirm risk/compliance responses.

## 6) Move to Assist/Auto Safely

Progression:

1. `mode = "assist"`
2. `mode = "auto"` with `[agent].dry_run = true`
3. disable dry-run only after behavior is stable and expected

Keep `[alpaca].env = "paper"` through validation.

## 7) Enable Strategy Overseer (Optional)

Set:

- `[strategy].enabled = true`

Then in TUI:

- `strategy run`
- review Strategy tab output
- optionally `strategy approve <PLAN_ID>`

## 8) Validation Commands

```bash
go test ./...
go build ./cmd/helix
```

## 9) Troubleshooting Quick Hits

- Auth errors:
  - verify paper keys
  - verify `[alpaca].env = "paper"`
  - check env vars are not stale/mis-set
- No autonomous activity:
  - check `mode`
  - check `[agent].dry_run`
  - check watchlist symbols and risk/compliance rejections
- LLM issues:
  - confirm `OPENAI_API_KEY` or `[agent.llm].api_key`
  - increase `[agent.llm].timeout` for slower responses

## 10) Next Docs

- Daily operations and incident handling: `docs/operator-playbook.md`
- Architecture and data flow: `docs/architecture.md`
