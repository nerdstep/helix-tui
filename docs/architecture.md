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
    CTX[Decision Context Gate<br/>hash + change detection]
    QSC[runtime quote stream controller]
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
    APP --> QSC
    RUNNER --> CTX
    CTX --> AGENT
    AGENT --> ENGINE
    TUI --> ENGINE
    QSC --> ENGINE
    QSC --> ABRK
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
        A->>E: Sync snapshot + quotes
        A->>A: Build context hash
        alt Context changed (or force window reached)
            A->>A: Invoke agent
            A->>E: ExecuteIntent(intent)
        else Context unchanged
            A->>E: Record skip event
        end
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

## Quote Data Flow

```mermaid
sequenceDiagram
    participant W as Runtime
    participant Q as Quote Stream Controller
    participant E as Engine quote cache
    participant B as Alpaca Broker
    participant S as Alpaca WS feed
    participant R as Runner/TUI

    W->>Q: Start with watchlist symbols
    Q->>B: StreamQuotes(symbols)
    B->>S: Subscribe quotes via websocket
    S-->>B: Quote updates
    B-->>Q: domain.Quote channel
    Q->>E: UpsertQuote(quote)
    R->>E: GetQuote(symbol)
    E-->>R: Fresh cached quote (fallback to REST when stale/missing)
    Q->>Q: On watchlist change: cancel + resubscribe
```

## Autonomous Decision Loop

```mermaid
flowchart TD
    TICK[Interval tick]
    SYNC[Engine.SyncQuiet]
    BUILD[Build decision context<br/>watchlist/account/positions/orders/quotes]
    HASH[Hash context]
    CHANGED{Changed since last successful<br/>agent call?}
    FORCE{Force refresh window reached?}
    CALL[Invoke agent ProposeTrades]
    SKIP[Skip agent call<br/>emit agent_context_unchanged]
    EXEC[Risk-gated execution path]

    TICK --> SYNC --> BUILD --> HASH --> CHANGED
    CHANGED -->|Yes| CALL --> EXEC
    CHANGED -->|No| FORCE
    FORCE -->|Yes| CALL --> EXEC
    FORCE -->|No| SKIP
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
