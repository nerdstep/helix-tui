package heuristic

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"helix-tui/internal/domain"
)

type Agent struct {
	broker     domain.Broker
	minMovePct float64
	orderQty   float64
	lastQuotes map[string]float64
}

func New(broker domain.Broker, minMovePct, orderQty float64) *Agent {
	if minMovePct <= 0 {
		minMovePct = 0.01
	}
	if orderQty <= 0 {
		orderQty = 1
	}
	return &Agent{
		broker:     broker,
		minMovePct: minMovePct,
		orderQty:   orderQty,
		lastQuotes: map[string]float64{},
	}
}

func (a *Agent) ProposeTrades(ctx context.Context, input domain.AgentInput) ([]domain.TradeIntent, error) {
	if len(input.Watchlist) == 0 {
		return nil, nil
	}

	positions := map[string]domain.Position{}
	for _, p := range input.Snapshot.Positions {
		positions[strings.ToUpper(p.Symbol)] = p
	}

	openBySymbol := map[string]struct{}{}
	for _, o := range input.Snapshot.Orders {
		if o.Status == domain.OrderStatusNew || o.Status == domain.OrderStatusAccepted || o.Status == domain.OrderStatusPartially {
			openBySymbol[strings.ToUpper(o.Symbol)] = struct{}{}
		}
	}

	quotesBySymbol := make(map[string]domain.Quote, len(input.Quotes))
	for _, q := range input.Quotes {
		symbol := strings.ToUpper(strings.TrimSpace(q.Symbol))
		if symbol == "" {
			continue
		}
		quotesBySymbol[symbol] = q
	}

	type scoredIntent struct {
		intent domain.TradeIntent
		score  float64
	}
	out := make([]scoredIntent, 0, len(input.Watchlist))
	seen := map[string]struct{}{}

	for _, rawSymbol := range input.Watchlist {
		symbol := strings.ToUpper(strings.TrimSpace(rawSymbol))
		if symbol == "" {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		if _, hasOpenOrder := openBySymbol[symbol]; hasOpenOrder {
			continue
		}

		quote, ok := quotesBySymbol[symbol]
		if !ok {
			q, err := a.broker.GetQuote(ctx, symbol)
			if err != nil {
				continue
			}
			quote = q
		}
		if quote.Last <= 0 {
			continue
		}

		prev, ok := a.lastQuotes[symbol]
		a.lastQuotes[symbol] = quote.Last
		if !ok || prev <= 0 {
			continue
		}

		movePct := (quote.Last - prev) / prev
		absMove := math.Abs(movePct)
		if absMove < a.minMovePct {
			continue
		}

		if movePct <= -a.minMovePct {
			conf := confidence(absMove, a.minMovePct)
			out = append(out, scoredIntent{
				intent: domain.TradeIntent{
					Symbol:          symbol,
					Side:            domain.SideBuy,
					Qty:             a.orderQty,
					OrderType:       domain.OrderTypeMarket,
					Rationale:       fmt.Sprintf("price moved %.2f%% down since last sample", movePct*100),
					Confidence:      conf,
					ExpectedGainPct: absMove * 100,
				},
				score: absMove,
			})
		}

		if movePct >= a.minMovePct {
			pos := positions[symbol]
			if pos.Qty <= 0 {
				continue
			}
			qty := math.Min(pos.Qty, a.orderQty)
			conf := confidence(absMove, a.minMovePct)
			out = append(out, scoredIntent{
				intent: domain.TradeIntent{
					Symbol:          symbol,
					Side:            domain.SideSell,
					Qty:             qty,
					OrderType:       domain.OrderTypeMarket,
					Rationale:       fmt.Sprintf("price moved %.2f%% up since last sample", movePct*100),
					Confidence:      conf,
					ExpectedGainPct: absMove * 100,
				},
				score: absMove,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })
	intents := make([]domain.TradeIntent, 0, len(out))
	for _, s := range out {
		intents = append(intents, s.intent)
	}
	return intents, nil
}

func confidence(absMove, minMove float64) float64 {
	if minMove <= 0 {
		return 0.5
	}
	c := absMove / (2 * minMove)
	if c > 0.99 {
		return 0.99
	}
	if c < 0.10 {
		return 0.10
	}
	return c
}
