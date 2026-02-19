# Architecture

## Component Diagram

```mermaid
flowchart LR
    U[Operator]
    CLI[cmd/helix CLI]
    CFG[configfile + env + flags]
    APP[internal/app.NewSystem]
    TUI[internal/tui]
    RUNNER[autonomy.Runner]
    AGENT[agent/heuristic.Agent]
    ENGINE[engine.Engine]
    RISK[engine.RiskGate]
    KEYRING[credentials keyring]
    AB[(Broker Interface)]
    PBRK[paper Broker]
    ABRK[alpaca Broker]
    ALPACA[(Alpaca Trade/Data APIs)]

    U --> CLI
    CLI --> CFG
    CFG --> APP
    CFG --> KEYRING
    APP --> ENGINE
    APP --> TUI
    APP --> RUNNER
    RUNNER --> AGENT
    AGENT --> ENGINE
    TUI --> ENGINE
    ENGINE --> RISK
    ENGINE --> AB
    AB --> PBRK
    AB --> ABRK
    ABRK --> ALPACA
    APP --> ABRK
```

## Order Data Flow (Manual and Autonomous)

```mermaid
sequenceDiagram
    participant Op as Operator
    participant T as TUI/CLI
    participant A as Agent Runner
    participant E as Engine
    participant R as RiskGate
    participant B as Broker (paper/alpaca)
    participant X as Alpaca API

    alt Manual Command
        Op->>T: buy/sell command
        T->>E: PlaceOrder(request)
    else Autonomous Cycle
        A->>E: ExecuteIntent(intent)
    end

    E->>B: GetQuote(symbol)
    B-->>E: Quote
    E->>R: Evaluate(request, quote)
    R-->>E: allow/reject

    alt Allowed
        E->>B: PlaceOrder(request)
        alt Alpaca broker
            B->>X: submit order
            X-->>B: order status/update
        end
        B-->>E: Order/TradeUpdate
        E-->>T: Snapshot + events
    else Rejected
        R-->>E: policy violation
        E-->>T: agent_intent_rejected / error event
    end
```

## Watchlist Flow

```mermaid
flowchart TD
    START[Startup]
    LOADCFG[Load config + flags + env]
    PULLALPACA[If broker=alpaca:<br/>pull watchlist from Alpaca API]
    LOCALWATCH[If broker=paper:<br/>use local watchlist from config/flags]
    ALLOW[Add watchlist symbols to risk allowlist]
    RUN[Run TUI/agent]

    START --> LOADCFG --> PULLALPACA --> ALLOW --> RUN
    START --> LOADCFG --> LOCALWATCH --> ALLOW --> RUN

    RUN -->|watch add/remove (alpaca)| PUSH[Push to Alpaca watchlist API]
    RUN -->|watch sync/pull (alpaca)| PULL[Pull remote Alpaca watchlist]
```
