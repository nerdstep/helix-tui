package strategy

import (
	"context"
	"time"

	"helix-tui/internal/domain"
)

type Input struct {
	GeneratedAt        time.Time
	Objective          string
	MaxRecommendations int
	Watchlist          []string
	CurrentPlan        *CurrentPlan
	Snapshot           domain.Snapshot
	Quotes             []domain.Quote
	RecentEvents       []domain.Event
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
