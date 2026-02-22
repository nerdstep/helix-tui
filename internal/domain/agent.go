package domain

type AgentInput struct {
	Mode        Mode
	Watchlist   []string
	Snapshot    Snapshot
	Compliance  *ComplianceStatus
	Quotes      []Quote
	QuoteErrors []string
}
