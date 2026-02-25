package tui

import "time"

type StrategyRecommendationView struct {
	Symbol       string
	Bias         string
	Confidence   float64
	EntryMin     float64
	EntryMax     float64
	TargetPrice  float64
	StopPrice    float64
	MaxNotional  float64
	Thesis       string
	Invalidation string
	Priority     int
}

type StrategyPlanView struct {
	ID              uint
	GeneratedAt     time.Time
	UpdatedAt       time.Time
	Status          string
	AnalystModel    string
	PromptVersion   string
	Watchlist       []string
	Summary         string
	Confidence      float64
	Recommendations []StrategyRecommendationView
}

type StrategyChatThreadView struct {
	ID            uint
	Title         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastMessageAt time.Time
}

type StrategyChatMessageView struct {
	ID        uint
	ThreadID  uint
	Role      string
	Content   string
	Model     string
	CreatedAt time.Time
}

type StrategyChatView struct {
	ActiveThreadID uint
	Threads        []StrategyChatThreadView
	Messages       []StrategyChatMessageView
}

type StrategySteeringView struct {
	Version             uint64
	Source              string
	RiskProfile         string
	MinConfidence       float64
	MaxPositionNotional float64
	Horizon             string
	Objective           string
	PreferredSymbols    []string
	ExcludedSymbols     []string
	Hash                string
	UpdatedAt           time.Time
}

type StrategyProposalView struct {
	ID                  uint
	ThreadID            uint
	Source              string
	Kind                string
	Status              string
	Rationale           string
	AddSymbols          []string
	RemoveSymbols       []string
	RiskProfile         string
	MinConfidence       float64
	MaxPositionNotional float64
	Horizon             string
	Objective           string
	PreferredSymbols    []string
	ExcludedSymbols     []string
	Hash                string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type StrategySnapshot struct {
	Active   *StrategyPlanView
	Recent   []StrategyPlanView
	Steering *StrategySteeringView
	Chat     StrategyChatView
	Proposals []StrategyProposalView
}
