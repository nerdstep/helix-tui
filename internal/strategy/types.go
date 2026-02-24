package strategy

import (
	"context"
	"time"

	"helix-tui/internal/domain"
)

type Input struct {
	GeneratedAt        time.Time
	MaxRecommendations int
	Watchlist          []string
	Steering           *SteeringContext
	CurrentPlan        *CurrentPlan
	Snapshot           domain.Snapshot
	Quotes             []domain.Quote
	RecentEvents       []domain.Event
}

type SteeringContext struct {
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

type CurrentPlan struct {
	ID              uint
	GeneratedAt     time.Time
	Status          string
	Summary         string
	Confidence      float64
	Recommendations []Recommendation
}

type Recommendation struct {
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

type Plan struct {
	NoChange        bool
	Summary         string
	Confidence      float64
	Recommendations []Recommendation
}

type Analyst interface {
	BuildPlan(ctx context.Context, input Input) (Plan, error)
}

type ChatMessage struct {
	Role      string
	Content   string
	CreatedAt time.Time
}

type ChatInput struct {
	GeneratedAt  time.Time
	Watchlist    []string
	Snapshot     domain.Snapshot
	Quotes       []domain.Quote
	CurrentPlan  *CurrentPlan
	Messages     []ChatMessage
	RecentEvents []domain.Event
}

type ChatReply struct {
	Content string
	Model   string
}

type Copilot interface {
	Reply(ctx context.Context, input ChatInput) (ChatReply, error)
}
