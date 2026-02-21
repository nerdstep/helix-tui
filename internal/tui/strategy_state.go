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

type StrategySnapshot struct {
	Active *StrategyPlanView
	Recent []StrategyPlanView
}
