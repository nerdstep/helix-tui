package llm

import (
	"context"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

func TestNewRequiresAPIKey(t *testing.T) {
	_, err := New(testBroker{}, Config{})
	if err == nil {
		t.Fatalf("expected api key error")
	}
}

func TestProposeTradesParsesLLMIntents(t *testing.T) {
	agent, err := newWithClient(testBroker{
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100, Bid: 99, Ask: 101, Time: time.Now().UTC()},
		},
	}, Config{
		APIKey: "test-key",
		Model:  "test-model",
	}, staticChatClient{
		content: `{"intents":[{"symbol":"aapl","side":"buy","qty":2,"order_type":"market","confidence":0.8,"rationale":"strong setup"}]}`,
	})
	if err != nil {
		t.Fatalf("newWithClient failed: %v", err)
	}

	intents, err := agent.ProposeTrades(context.Background(), domain.AgentInput{
		Mode:      domain.ModeAuto,
		Watchlist: []string{"AAPL"},
		Snapshot:  domain.Snapshot{},
		Objective: "Trade momentum reversals.",
	})
	if err != nil {
		t.Fatalf("ProposeTrades failed: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected one intent, got %d", len(intents))
	}
	intent := intents[0]
	if intent.Symbol != "AAPL" || intent.Side != domain.SideBuy || intent.Qty != 2 {
		t.Fatalf("unexpected intent: %#v", intent)
	}
}

func TestProposeTradesDropsSymbolsOutsideWatchlist(t *testing.T) {
	agent, err := newWithClient(testBroker{
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100, Bid: 99, Ask: 101, Time: time.Now().UTC()},
		},
	}, Config{
		APIKey: "test-key",
		Model:  "test-model",
	}, staticChatClient{
		content: `{"intents":[{"symbol":"MSFT","side":"buy","qty":2,"order_type":"market","confidence":0.8}]}`,
	})
	if err != nil {
		t.Fatalf("newWithClient failed: %v", err)
	}

	intents, err := agent.ProposeTrades(context.Background(), domain.AgentInput{
		Mode:      domain.ModeAuto,
		Watchlist: []string{"AAPL"},
		Snapshot:  domain.Snapshot{},
	})
	if err != nil {
		t.Fatalf("ProposeTrades failed: %v", err)
	}
	if len(intents) != 0 {
		t.Fatalf("expected no intents, got %#v", intents)
	}
}

func TestParseIntentsInvalidJSON(t *testing.T) {
	_, err := parseIntents("not-json", []string{"AAPL"})
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "response preview") {
		t.Fatalf("expected response preview in error, got: %v", err)
	}
}

func TestParseIntentsFromMarkdownCodeFence(t *testing.T) {
	raw := "Analysis:\n```json\n{\"intents\":[{\"symbol\":\"AAPL\",\"side\":\"buy\",\"qty\":1,\"order_type\":\"market\"}]}\n```"
	intents, err := parseIntents(raw, []string{"AAPL"})
	if err != nil {
		t.Fatalf("parseIntents failed: %v", err)
	}
	if len(intents) != 1 || intents[0].Symbol != "AAPL" {
		t.Fatalf("unexpected intents: %#v", intents)
	}
}

func TestParseIntentsFromMixedText(t *testing.T) {
	raw := "Action plan follows.\n{\"intents\":[{\"symbol\":\"AAPL\",\"side\":\"sell\",\"qty\":2,\"order_type\":\"market\"}]}\nThank you."
	intents, err := parseIntents(raw, []string{"AAPL"})
	if err != nil {
		t.Fatalf("parseIntents failed: %v", err)
	}
	if len(intents) != 1 || intents[0].Side != domain.SideSell || intents[0].Qty != 2 {
		t.Fatalf("unexpected intents: %#v", intents)
	}
}

func TestParseIntentsFromRawArray(t *testing.T) {
	raw := `[{"symbol":"AAPL","side":"buy","qty":1,"order_type":"market"}]`
	intents, err := parseIntents(raw, []string{"AAPL"})
	if err != nil {
		t.Fatalf("parseIntents failed: %v", err)
	}
	if len(intents) != 1 || intents[0].Symbol != "AAPL" {
		t.Fatalf("unexpected intents: %#v", intents)
	}
}

type staticChatClient struct {
	content string
	err     error
}

func (s staticChatClient) Complete(context.Context, string, string, string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.content, nil
}

type testBroker struct {
	quotes map[string]domain.Quote
}

func (b testBroker) GetAccount(context.Context) (domain.Account, error) { return domain.Account{}, nil }
func (b testBroker) GetPositions(context.Context) ([]domain.Position, error) {
	return nil, nil
}
func (b testBroker) GetOpenOrders(context.Context) ([]domain.Order, error) { return nil, nil }
func (b testBroker) GetQuote(_ context.Context, symbol string) (domain.Quote, error) {
	if q, ok := b.quotes[symbol]; ok {
		return q, nil
	}
	return domain.Quote{Symbol: symbol}, nil
}
func (b testBroker) PlaceOrder(context.Context, domain.OrderRequest) (domain.Order, error) {
	return domain.Order{}, nil
}
func (b testBroker) CancelOrder(context.Context, string) error { return nil }
func (b testBroker) StreamTradeUpdates(context.Context) (<-chan domain.TradeUpdate, error) {
	ch := make(chan domain.TradeUpdate)
	close(ch)
	return ch, nil
}
