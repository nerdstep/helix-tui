package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

type Config struct {
	APIKey       string
	BaseURL      string
	Model        string
	Timeout      time.Duration
	SystemPrompt string
}

type Agent struct {
	broker       domain.Broker
	client       chatClient
	model        string
	timeout      time.Duration
	systemPrompt string
	maxWatchlist int
	maxEvents    int
}

type chatClient interface {
	Complete(ctx context.Context, model, systemPrompt, userPrompt string) (string, error)
}

func New(broker domain.Broker, cfg Config) (*Agent, error) {
	return newWithClient(broker, cfg, newOpenAIChatClient(cfg.APIKey, cfg.BaseURL))
}

func newWithClient(broker domain.Broker, cfg Config, client chatClient) (*Agent, error) {
	if broker == nil {
		return nil, fmt.Errorf("llm agent requires a broker")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("llm api key is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-4.1-mini"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	return &Agent{
		broker:       broker,
		client:       client,
		model:        model,
		timeout:      timeout,
		systemPrompt: systemPrompt,
		maxWatchlist: 12,
		maxEvents:    40,
	}, nil
}

func (a *Agent) ProposeTrades(ctx context.Context, input domain.AgentInput) ([]domain.TradeIntent, error) {
	watchlist := symbols.Normalize(input.Watchlist)
	if len(watchlist) == 0 {
		return nil, nil
	}
	if len(watchlist) > a.maxWatchlist {
		watchlist = watchlist[:a.maxWatchlist]
	}

	quotes := make([]quoteInput, 0, len(watchlist))
	quoteErrors := make([]string, 0)
	for _, symbol := range watchlist {
		q, err := a.broker.GetQuote(ctx, symbol)
		if err != nil {
			quoteErrors = append(quoteErrors, fmt.Sprintf("%s: %v", symbol, err))
			continue
		}
		quotes = append(quotes, quoteInput{
			Symbol: q.Symbol,
			Bid:    q.Bid,
			Ask:    q.Ask,
			Last:   q.Last,
			Time:   q.Time.UTC().Format(time.RFC3339),
		})
	}

	payload := llmInput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Mode:        string(input.Mode),
		Objective:   strings.TrimSpace(input.Objective),
		Watchlist:   watchlist,
		Account:     input.Snapshot.Account,
		Positions:   input.Snapshot.Positions,
		OpenOrders:  openOrdersForPrompt(input.Snapshot.Orders),
		Quotes:      quotes,
		QuoteErrors: quoteErrors,
		RecentEvents: recentEventsForPrompt(
			input.Snapshot.Events,
			a.maxEvents,
		),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal llm input: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	content, err := a.client.Complete(callCtx, a.model, a.systemPrompt, string(body))
	if err != nil {
		return nil, err
	}
	return parseIntents(content, watchlist)
}

const defaultSystemPrompt = "You are a conservative US equities trading research assistant. " +
	"Use only the provided JSON context. " +
	"Return strict JSON: {\"intents\":[{\"symbol\":\"AAPL\",\"side\":\"buy|sell\",\"qty\":1.0,\"order_type\":\"market|limit\",\"limit_price\":123.45,\"confidence\":0.0,\"rationale\":\"...\"}]}. " +
	"Only propose watchlist symbols. Keep qty positive. Return {\"intents\":[]} when uncertain."

type llmInput struct {
	GeneratedAt  string            `json:"generated_at"`
	Mode         string            `json:"mode"`
	Objective    string            `json:"objective"`
	Watchlist    []string          `json:"watchlist"`
	Account      domain.Account    `json:"account"`
	Positions    []domain.Position `json:"positions"`
	OpenOrders   []orderInput      `json:"open_orders"`
	Quotes       []quoteInput      `json:"quotes"`
	QuoteErrors  []string          `json:"quote_errors"`
	RecentEvents []eventInput      `json:"recent_events"`
}

type orderInput struct {
	Symbol string             `json:"symbol"`
	Side   domain.Side        `json:"side"`
	Qty    float64            `json:"qty"`
	Status domain.OrderStatus `json:"status"`
	Type   domain.OrderType   `json:"type"`
}

type quoteInput struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	Last   float64 `json:"last"`
	Time   string  `json:"time"`
}

type eventInput struct {
	Time    string `json:"time"`
	Type    string `json:"type"`
	Details string `json:"details"`
}

func openOrdersForPrompt(orders []domain.Order) []orderInput {
	out := make([]orderInput, 0, len(orders))
	for _, o := range orders {
		out = append(out, orderInput{
			Symbol: o.Symbol,
			Side:   o.Side,
			Qty:    o.Qty,
			Status: o.Status,
			Type:   o.Type,
		})
	}
	return out
}

func recentEventsForPrompt(events []domain.Event, limit int) []eventInput {
	if limit <= 0 || len(events) <= limit {
		return toEventInputs(events)
	}
	return toEventInputs(events[len(events)-limit:])
}

func toEventInputs(events []domain.Event) []eventInput {
	out := make([]eventInput, 0, len(events))
	for _, e := range events {
		out = append(out, eventInput{
			Time:    e.Time.UTC().Format(time.RFC3339),
			Type:    e.Type,
			Details: e.Details,
		})
	}
	return out
}

type llmOutput struct {
	Intents []intentOutput `json:"intents"`
}

type intentOutput struct {
	Symbol     string   `json:"symbol"`
	Side       string   `json:"side"`
	Qty        float64  `json:"qty"`
	OrderType  string   `json:"order_type"`
	LimitPrice *float64 `json:"limit_price"`
	Confidence float64  `json:"confidence"`
	Rationale  string   `json:"rationale"`
}

func parseIntents(raw string, watchlist []string) ([]domain.TradeIntent, error) {
	out, err := decodeLLMOutput(raw)
	if err != nil {
		return nil, fmt.Errorf("parse llm output: %w", err)
	}
	if len(out.Intents) == 0 {
		return nil, nil
	}

	allowed := make(map[string]struct{}, len(watchlist))
	for _, s := range watchlist {
		allowed[s] = struct{}{}
	}

	intents := make([]domain.TradeIntent, 0, len(out.Intents))
	for _, intent := range out.Intents {
		normalized := symbols.Normalize([]string{intent.Symbol})
		if len(normalized) == 0 {
			continue
		}
		symbol := normalized[0]
		if _, ok := allowed[symbol]; !ok {
			continue
		}
		if intent.Qty <= 0 {
			continue
		}

		side := domain.Side(strings.ToLower(strings.TrimSpace(intent.Side)))
		if side != domain.SideBuy && side != domain.SideSell {
			continue
		}
		orderType := domain.OrderType(strings.ToLower(strings.TrimSpace(intent.OrderType)))
		switch orderType {
		case domain.OrderTypeMarket:
		case domain.OrderTypeLimit:
			if intent.LimitPrice == nil || *intent.LimitPrice <= 0 {
				continue
			}
		default:
			orderType = domain.OrderTypeMarket
		}

		conf := intent.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}
		rationale := strings.TrimSpace(intent.Rationale)
		if rationale == "" {
			rationale = "llm-generated intent"
		}
		intents = append(intents, domain.TradeIntent{
			Symbol:     symbol,
			Side:       side,
			Qty:        intent.Qty,
			OrderType:  orderType,
			LimitPrice: intent.LimitPrice,
			Confidence: conf,
			Rationale:  rationale,
		})
	}
	return intents, nil
}

var jsonFencePattern = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")

func decodeLLMOutput(raw string) (llmOutput, error) {
	candidates := collectJSONCandidates(raw)
	var lastErr error
	for _, candidate := range candidates {
		var out llmOutput
		if err := json.Unmarshal([]byte(candidate), &out); err == nil {
			return out, nil
		} else {
			lastErr = err
		}

		// Fallback for model responses that emit a raw array of intents.
		var intents []intentOutput
		if err := json.Unmarshal([]byte(candidate), &intents); err == nil {
			return llmOutput{Intents: intents}, nil
		}
	}
	return llmOutput{}, fmt.Errorf("%v (response preview: %q)", lastErr, previewResponse(raw))
}

func collectJSONCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"{}"}
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}

	add(raw)
	for _, block := range extractFencedBlocks(raw) {
		add(block)
		for _, object := range extractBalancedJSONObjects(block) {
			add(object)
		}
	}
	for _, object := range extractBalancedJSONObjects(raw) {
		add(object)
	}
	return out
}

func extractFencedBlocks(raw string) []string {
	matches := jsonFencePattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			out = append(out, match[1])
		}
	}
	return out
}

func extractBalancedJSONObjects(raw string) []string {
	if raw == "" {
		return nil
	}
	out := make([]string, 0, 2)
	b := []byte(raw)
	for start := 0; start < len(b); start++ {
		if b[start] != '{' {
			continue
		}
		depth := 0
		inString := false
		escaped := false
		for i := start; i < len(b); i++ {
			ch := b[i]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				switch ch {
				case '\\':
					escaped = true
				case '"':
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					out = append(out, raw[start:i+1])
					start = i
					i = len(b) // break loop
				}
			}
		}
	}
	return out
}

func previewResponse(raw string) string {
	preview := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if preview == "" {
		return "(empty)"
	}
	const max = 180
	if len(preview) <= max {
		return preview
	}
	return preview[:max] + "..."
}

type openAIChatClient struct {
	client openai.Client
}

func newOpenAIChatClient(apiKey, baseURL string) *openAIChatClient {
	opts := []option.RequestOption{
		option.WithAPIKey(strings.TrimSpace(apiKey)),
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &openAIChatClient{
		client: openai.NewClient(opts...),
	}
}

func (c *openAIChatClient) Complete(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
		},
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
	})
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("openai response has no choices")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("openai response is empty")
	}
	return content, nil
}
