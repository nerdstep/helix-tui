package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

type fakeCopilotClient struct {
	lastModel        string
	lastInstructions string
	lastUserPrompt   string
	response         string
	err              error
}

func (f *fakeCopilotClient) Complete(_ context.Context, model, instructions, userPrompt string) (string, error) {
	f.lastModel = model
	f.lastInstructions = instructions
	f.lastUserPrompt = userPrompt
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func TestLLMCopilotReplyBuildsPayloadAndReturnsContent(t *testing.T) {
	fake := &fakeCopilotClient{
		response: "Suggested update:\n- tighten max_notional for volatile symbols.",
	}
	copilot, err := newLLMCopilotWithClient(LLMCopilotConfig{
		APIKey:       "test-key",
		Model:        "gpt-5-mini",
		SystemPrompt: "You are Helix strategy copilot.",
		HumanName:    "Justin",
		AgentName:    "Helix",
	}, fake)
	if err != nil {
		t.Fatalf("newLLMCopilotWithClient failed: %v", err)
	}

	reply, err := copilot.Reply(context.Background(), ChatInput{
		GeneratedAt: time.Now().UTC(),
		Watchlist:   []string{"AAPL", "MSFT"},
		Snapshot: domain.Snapshot{
			Account:   domain.Account{Cash: 1000, Equity: 1100},
			Positions: []domain.Position{{Symbol: "AAPL", Qty: 2, AvgCost: 150, LastPrice: 155}},
			Orders:    []domain.Order{{ID: "ord-1", Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeLimit}},
		},
		Quotes: []domain.Quote{{Symbol: "AAPL", Last: 155, Bid: 154.9, Ask: 155.1}},
		CurrentPlan: &CurrentPlan{
			ID:         4,
			Status:     "active",
			Summary:    "Focus on quality large caps.",
			Confidence: 0.71,
			Recommendations: []Recommendation{
				{Symbol: "AAPL", Bias: "buy", Confidence: 0.7, Priority: 1},
			},
		},
		Messages: []ChatMessage{
			{Role: "user", Content: "Should we rotate out of MSFT into semis?", CreatedAt: time.Now().UTC()},
		},
		RecentEvents: []domain.Event{
			{Time: time.Now().UTC(), Type: "strategy_plan_created", Details: "id=4 status=active recs=3"},
		},
	})
	if err != nil {
		t.Fatalf("Reply failed: %v", err)
	}
	if strings.TrimSpace(reply.Content) == "" {
		t.Fatalf("expected non-empty reply")
	}
	if reply.Model != "gpt-5-mini" {
		t.Fatalf("unexpected reply model: %q", reply.Model)
	}
	if len(reply.Proposals) != 0 {
		t.Fatalf("expected no proposals for plain text response, got %#v", reply.Proposals)
	}
	if !strings.Contains(fake.lastInstructions, "Identity context") {
		t.Fatalf("expected identity context in instructions, got %q", fake.lastInstructions)
	}
	if fake.lastModel != "gpt-5-mini" {
		t.Fatalf("unexpected model passed to client: %q", fake.lastModel)
	}
	if !strings.HasPrefix(fake.lastUserPrompt, "JSON input:\n") {
		t.Fatalf("expected JSON input prefix, got %q", fake.lastUserPrompt)
	}
	var payload strategyCopilotPayload
	raw := strings.TrimPrefix(fake.lastUserPrompt, "JSON input:\n")
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if len(payload.Watchlist) != 2 || payload.Watchlist[0] != "AAPL" {
		t.Fatalf("unexpected payload watchlist: %#v", payload.Watchlist)
	}
	if payload.CurrentPlan == nil || payload.CurrentPlan.ID != 4 {
		t.Fatalf("expected current_plan in payload, got %#v", payload.CurrentPlan)
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
		t.Fatalf("unexpected payload messages: %#v", payload.Messages)
	}
}

func TestLLMCopilotReplyPropagatesClientError(t *testing.T) {
	fake := &fakeCopilotClient{err: errors.New("boom")}
	copilot, err := newLLMCopilotWithClient(LLMCopilotConfig{
		APIKey: "test-key",
	}, fake)
	if err != nil {
		t.Fatalf("newLLMCopilotWithClient failed: %v", err)
	}
	_, err = copilot.Reply(context.Background(), ChatInput{
		GeneratedAt: time.Now().UTC(),
	})
	if err == nil || !strings.Contains(err.Error(), "strategy copilot request failed") {
		t.Fatalf("expected wrapped request error, got %v", err)
	}
}

func TestTruncateStrategyCopilotText(t *testing.T) {
	long := "a " + strings.Repeat("b", 128)
	got := truncateStrategyCopilotText(long, 20)
	if len(got) == 0 {
		t.Fatalf("expected truncated text")
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
}

func TestLLMCopilotReplyExtractsStructuredProposals(t *testing.T) {
	fake := &fakeCopilotClient{
		response: "Rotate risk lower this week.\n<helix_proposal>{\"watchlist_proposal\":{\"add\":[\"aapl\",\"msft\"],\"remove\":[\"tsla\"]},\"steering_proposal\":{\"risk_profile\":\"conservative\",\"min_confidence\":0.72,\"max_position_notional\":2500,\"horizon\":\"swing\",\"objective\":\"Protect downside.\",\"preferred_symbols\":[\"AAPL\"],\"excluded_symbols\":[\"GME\"]}}</helix_proposal>",
	}
	copilot, err := newLLMCopilotWithClient(LLMCopilotConfig{
		APIKey: "test-key",
		Model:  "gpt-5-mini",
	}, fake)
	if err != nil {
		t.Fatalf("newLLMCopilotWithClient failed: %v", err)
	}

	reply, err := copilot.Reply(context.Background(), ChatInput{GeneratedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("Reply failed: %v", err)
	}
	if !strings.Contains(reply.Content, "Rotate risk lower") {
		t.Fatalf("expected cleaned plain-text content, got %q", reply.Content)
	}
	if strings.Contains(reply.Content, "helix_proposal") {
		t.Fatalf("expected proposal block removed from visible content, got %q", reply.Content)
	}
	if len(reply.Proposals) != 2 {
		t.Fatalf("expected 2 proposals, got %#v", reply.Proposals)
	}
	if reply.Proposals[0].Kind != CopilotProposalKindWatchlist {
		t.Fatalf("expected first proposal kind watchlist, got %#v", reply.Proposals[0])
	}
	if len(reply.Proposals[0].AddSymbols) != 2 || reply.Proposals[0].AddSymbols[0] != "AAPL" {
		t.Fatalf("unexpected watchlist add symbols: %#v", reply.Proposals[0].AddSymbols)
	}
	if reply.Proposals[1].Kind != CopilotProposalKindSteering {
		t.Fatalf("expected second proposal kind steering, got %#v", reply.Proposals[1])
	}
	if reply.Proposals[1].RiskProfile != "conservative" {
		t.Fatalf("unexpected steering proposal: %#v", reply.Proposals[1])
	}
}
