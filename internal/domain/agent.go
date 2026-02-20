package domain

type AgentInput struct {
	Mode        Mode
	Watchlist   []string
	Snapshot    Snapshot
	Quotes      []Quote
	QuoteErrors []string
}
