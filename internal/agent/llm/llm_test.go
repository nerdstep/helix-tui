package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		content: `{"intents":[{"symbol":"aapl","side":"buy","qty":2,"order_type":"market","confidence":0.8,"expected_gain_pct":2.5,"rationale":"strong setup"}]}`,
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
	if len(intents) != 1 {
		t.Fatalf("expected one intent, got %d", len(intents))
	}
	intent := intents[0]
	if intent.Symbol != "AAPL" || intent.Side != domain.SideBuy || intent.Qty != 2 {
		t.Fatalf("unexpected intent: %#v", intent)
	}
	if intent.ExpectedGainPct != 2.5 {
		t.Fatalf("expected expected_gain_pct to be parsed, got %#v", intent)
	}
}

func TestParseIntentsMarketIgnoresLimitPrice(t *testing.T) {
	raw := `{"intents":[{"symbol":"AAPL","side":"buy","qty":1,"order_type":"market","limit_price":0,"expected_gain_pct":1.2}]}`
	intents, err := parseIntents(raw, []string{"AAPL"})
	if err != nil {
		t.Fatalf("parseIntents failed: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected one intent, got %#v", intents)
	}
	if intents[0].OrderType != domain.OrderTypeMarket {
		t.Fatalf("expected market order type, got %#v", intents[0])
	}
	if intents[0].LimitPrice != nil {
		t.Fatalf("expected market order limit price to be cleared, got %#v", intents[0].LimitPrice)
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

func TestProposeTradesIncludesRiskContextInPayload(t *testing.T) {
	capture := &captureChatClient{
		content: `{"intents":[]}`,
	}
	agent, err := newWithClient(testBroker{
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100, Bid: 99, Ask: 101, Time: time.Now().UTC()},
		},
	}, Config{
		APIKey:           "test-key",
		Model:            "test-model",
		MaxTradeNotional: 5000,
		MaxDayNotional:   20000,
	}, capture)
	if err != nil {
		t.Fatalf("newWithClient failed: %v", err)
	}

	_, err = agent.ProposeTrades(context.Background(), domain.AgentInput{
		Mode:      domain.ModeAuto,
		Watchlist: []string{"AAPL"},
		Snapshot:  domain.Snapshot{},
	})
	if err != nil {
		t.Fatalf("ProposeTrades failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(capture.userPrompt), &payload); err != nil {
		t.Fatalf("expected JSON payload in user prompt, got error: %v", err)
	}
	rawRisk, ok := payload["risk"]
	if !ok {
		t.Fatalf("expected risk section in payload")
	}
	risk, ok := rawRisk.(map[string]any)
	if !ok {
		t.Fatalf("expected risk object in payload, got %#v", rawRisk)
	}
	if got, ok := risk["max_trade_notional"].(float64); !ok || got != 5000 {
		t.Fatalf("unexpected max_trade_notional: %#v", risk["max_trade_notional"])
	}
	if got, ok := risk["max_day_notional"].(float64); !ok || got != 20000 {
		t.Fatalf("unexpected max_day_notional: %#v", risk["max_day_notional"])
	}
}

func TestRecentEventsForPromptFiltersAndLimits(t *testing.T) {
	base := time.Date(2026, 2, 20, 15, 0, 0, 0, time.UTC)
	events := []domain.Event{
		{Time: base.Add(1 * time.Second), Type: "agent_cycle_start", Details: "mode=auto watchlist=3"},
		{Time: base.Add(2 * time.Second), Type: "agent_context_summary", Details: "hash=abc"},
		{Time: base.Add(3 * time.Second), Type: "order_placed", Details: "buy RIVN 100"},
		{Time: base.Add(4 * time.Second), Type: "trade_update", Details: "123 status=new filled=0.00"},
		{Time: base.Add(5 * time.Second), Type: "trade_update", Details: "123 status=filled filled=100.00"},
		{Time: base.Add(6 * time.Second), Type: "agent_intent_executed", Details: "buy RIVN qty=100"},
		{Time: base.Add(7 * time.Second), Type: "agent_intent_executed", Details: "buy RIVN qty=100"},
		{Time: base.Add(8 * time.Second), Type: "agent_cycle_error", Details: "openai request failed: timeout"},
	}

	got := recentEventsForPrompt(events, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %#v", got)
	}
	if got[0].Type != "order_placed" {
		t.Fatalf("expected first event to be order_placed, got %#v", got[0])
	}
	if got[1].Type != "trade_update" || !strings.Contains(got[1].Details, "status=filled") {
		t.Fatalf("expected second event to be filled trade_update, got %#v", got[1])
	}
	if got[2].Type != "agent_intent_executed" {
		t.Fatalf("expected third event to be agent_intent_executed, got %#v", got[2])
	}
}

func TestRecentEventsForPromptTruncatesDetails(t *testing.T) {
	long := strings.Repeat("verylongdetail ", 40)
	events := []domain.Event{
		{Time: time.Now().UTC(), Type: "agent_intent_rejected", Details: long},
	}

	got := recentEventsForPrompt(events, 4)
	if len(got) != 1 {
		t.Fatalf("expected one event, got %#v", got)
	}
	if len(got[0].Details) > maxEventDetailChars+3 {
		t.Fatalf("expected truncated detail, got len=%d", len(got[0].Details))
	}
	if !strings.HasSuffix(got[0].Details, "...") {
		t.Fatalf("expected ellipsis suffix, got %#v", got[0].Details)
	}
}

func TestProposeTradesRetriesDeadlineExceeded(t *testing.T) {
	client := &sequenceChatClient{
		responses: []chatResponse{
			{err: fmt.Errorf("openai request failed: %w", context.DeadlineExceeded)},
			{content: `{"intents":[{"symbol":"AAPL","side":"buy","qty":1,"order_type":"market","expected_gain_pct":1.2}]}`},
		},
	}
	agent, err := newWithClient(testBroker{
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100, Bid: 99, Ask: 101, Time: time.Now().UTC()},
		},
	}, Config{
		APIKey:  "test-key",
		Model:   "test-model",
		Timeout: 20 * time.Second,
	}, client)
	if err != nil {
		t.Fatalf("newWithClient failed: %v", err)
	}

	intents, err := agent.ProposeTrades(context.Background(), domain.AgentInput{
		Mode:      domain.ModeAuto,
		Watchlist: []string{"AAPL"},
	})
	if err != nil {
		t.Fatalf("ProposeTrades failed: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected one intent, got %#v", intents)
	}
	if client.calls() != 2 {
		t.Fatalf("expected retry to call client twice, got %d", client.calls())
	}
}

func TestProposeTradesNoRetryOnNonRetryableError(t *testing.T) {
	client := &sequenceChatClient{
		responses: []chatResponse{
			{err: errors.New("openai request failed: 400 bad request")},
		},
	}
	agent, err := newWithClient(testBroker{
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100, Bid: 99, Ask: 101, Time: time.Now().UTC()},
		},
	}, Config{
		APIKey:  "test-key",
		Model:   "test-model",
		Timeout: 20 * time.Second,
	}, client)
	if err != nil {
		t.Fatalf("newWithClient failed: %v", err)
	}

	_, err = agent.ProposeTrades(context.Background(), domain.AgentInput{
		Mode:      domain.ModeAuto,
		Watchlist: []string{"AAPL"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if client.calls() != 1 {
		t.Fatalf("expected non-retryable error to call client once, got %d", client.calls())
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

type captureChatClient struct {
	content      string
	err          error
	model        string
	systemPrompt string
	userPrompt   string
}

type chatResponse struct {
	content string
	err     error
}

type sequenceChatClient struct {
	responses []chatResponse
	callCount int
}

func (c *sequenceChatClient) Complete(_ context.Context, _, _, _ string) (string, error) {
	c.callCount++
	idx := c.callCount - 1
	if idx < 0 || idx >= len(c.responses) {
		return "", errors.New("unexpected extra call")
	}
	resp := c.responses[idx]
	return resp.content, resp.err
}

func (c *sequenceChatClient) calls() int {
	return c.callCount
}

func (c *captureChatClient) Complete(_ context.Context, model, systemPrompt, userPrompt string) (string, error) {
	c.model = model
	c.systemPrompt = systemPrompt
	c.userPrompt = userPrompt
	if c.err != nil {
		return "", c.err
	}
	return c.content, nil
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
