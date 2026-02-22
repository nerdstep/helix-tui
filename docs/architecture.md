# Architecture

This document describes the current runtime architecture of `helix-tui`.

## Runtime Scope

- Runtime broker path: Alpaca (`[alpaca].env = paper|live`)
- Config-first runtime (`config.toml` + env overrides + minimal flags)
- Safety pipeline: `RiskGate` -> `ComplianceGate` -> broker execution
- Autonomous execution optionally constrained by active strategy plan

`internal/broker/paper` remains in the codebase for deterministic tests.

## High-Level Components

```mermaid
flowchart LR
    OP[Operator]
    CLI[cmd/helix]
    CFG["Config Loader<br/>config + env + flags"]
    APP[app.NewSystem]
    TUI[TUI]
    RUN[autonomy.Runner]
    STRAT[strategy.Runner]
    AGENT["Execution Agent<br/>heuristic | llm"]
    ANALYST["Strategy Analyst<br/>llm"]
    ENG[engine.Engine]
    RISK[RiskGate]
    COMP[ComplianceGate]
    BROKER[Alpaca Broker]
    ALP[(Alpaca APIs)]
    DB[(SQLite)]
    EVT[runtime event persistor]
    QSC[runtime quote stream controller]

    OP --> CLI --> CFG --> APP
    APP --> TUI
    APP --> RUN
    APP --> STRAT
    APP --> QSC
    APP --> EVT
    RUN --> AGENT --> ENG
    STRAT --> ANALYST
    TUI --> ENG
    ENG --> RISK --> COMP --> BROKER --> ALP
    QSC --> BROKER
    QSC --> ENG
    ENG --> EVT --> DB
    STRAT --> DB
    RUN --> DB
```

## Startup/Data Initialization

```mermaid
flowchart TD
    START[Process start]
    LOAD[Load config.toml]
    ENV[Apply env overrides]
    VALIDATE[Normalize + validate]
    BUILD[Build alpaca broker + engine]
    WATCH[PULL alpaca watchlist 'helix-tui']
    ALLOW[Watchlist -> risk allowlist]
    SYNC[Initial engine sync]
    RUNNERS[Start quote stream + optional runners]
    READY[TUI/headless running]

    START --> LOAD --> ENV --> VALIDATE --> BUILD --> WATCH --> ALLOW --> SYNC --> RUNNERS --> READY
```

## Order Execution Pipeline (Manual + Auto)

```mermaid
sequenceDiagram
    participant U as User/Runner
    participant E as Engine
    participant R as RiskGate
    participant C as ComplianceGate
    participant B as Alpaca Broker
    participant A as Alpaca API

    U->>E: PlaceOrder(request)
    E->>B: GetQuote(symbol)
    B-->>E: Quote
    E->>R: Evaluate(request, quote)
    R-->>E: allow/reject
    E->>C: Evaluate(request, quote, snapshot)
    C-->>E: allow/reject

    alt allowed
      E->>B: PlaceOrder(request)
      B->>A: submit order
      A-->>B: order/trade updates
      B-->>E: domain.Order / TradeUpdate
    else rejected
      E-->>U: rejection event/error
    end
```

## Autonomous Decision Loop

```mermaid
flowchart TD
    TICK[Cycle tick]
    LOWP[Low-power state check]
    SYNC[SyncQuiet + snapshot]
    QUOTES[Collect quotes]
    HASH[Hash decision context]
    CHANGED{Context changed?}
    FORCE{Force-refresh window reached?}
    CALL[Agent.ProposeTrades]
    FILTER[Per-cycle max intents + policy checks]
    EXEC[Place orders through engine]
    SKIP[Skip call + emit context unchanged]

    TICK --> LOWP
    LOWP -->|idle| SKIP
    LOWP -->|active/warmup| SYNC --> QUOTES --> HASH --> CHANGED
    CHANGED -->|yes| CALL --> FILTER --> EXEC
    CHANGED -->|no| FORCE
    FORCE -->|yes| CALL
    FORCE -->|no| SKIP
```

## Strategy Analyst Loop

```mermaid
sequenceDiagram
    participant S as strategy.Runner
    participant D as SQLite strategy tables
    participant L as LLM Analyst
    participant E as Engine
    participant A as autonomy.Runner

    S->>D: GetLatestPlan / GetActivePlan
    alt plan still fresh and not forced
      S-->>S: skip cycle
    else run cycle
      S->>E: Sync + snapshot + quotes + events
      S->>L: BuildPlan(input)
      L-->>S: Plan (or no_change)
      alt no_change
        S-->>E: strategy_plan_unchanged event
      else new plan
        S->>D: Create plan + recommendations
        alt auto_activate=true
          S->>D: Set active
        end
        S-->>E: strategy_plan_created event
      end
    end
    A->>D: Get active plan constraints
    A-->>A: enforce symbol/bias/max_notional
```

## Event Persistence + LLM Context

```mermaid
sequenceDiagram
    participant E as Engine Events
    participant P as Event Persistor
    participant D as SQLite trade_events
    participant R as autonomy.Runner
    participant L as LLM Agent

    E->>P: relevant event emitted
    P->>D: append batch (transaction)
    R->>D: ListRecent(N)
    D-->>R: recent persisted events
    R->>L: context payload (snapshot + quotes + risk + identity + recent events)
```

## Quote Streaming

```mermaid
flowchart LR
    WS[Alpaca quote websocket] --> AB[alpaca broker stream]
    AB --> QSC[quote stream controller]
    QSC --> CACHE[engine quote cache]
    RUN[runner/tui quote read] --> CACHE
    RUN -->|fallback when stale/missing| REST[alpaca latest quote REST]
```

## Safety Boundaries

- Watchlist is the effective execution allowlist.
- All order paths (manual/TUI/autonomous) go through the same engine gates.
- Compliance checks run after risk checks before broker submission.
- Autonomous execution can be globally dampened by low-power mode.
- Strategy policy can further restrict autonomous execution decisions.

## Compliance Reconciliation

- On every engine sync, compliance posture is reconciled against broker account state.
- Engine emits:
  - `compliance_posture` when posture changes (account type, PDT flags/counters, guard settings, unsettled estimates).
  - `compliance_drift_detected` / `compliance_drift_cleared` when local unsettled-proceeds estimates diverge from broker-implied unsettled funds.
- System tab surfaces posture and drift summaries for operators.
- Autonomous agent context now includes a structured `compliance` section, so LLM decisions can account for broker-reported PDT state and drift.
