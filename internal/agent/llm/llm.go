package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"

	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

const (
	defaultMaxPromptEvents  = 8
	maxEventDetailChars     = 140
	maxRejectionReasonChars = 280
)

type Config struct {
	APIKey           string
	BaseURL          string
	Model            string
	Timeout          time.Duration
	SystemPrompt     string
	MaxTradeNotional float64
	MaxDayNotional   float64
	MinGainPct       float64
	HumanName        string
	HumanAlias       string
	AgentName        string
}

type Agent struct {
	broker       domain.Broker
	client       chatClient
	model        string
	timeout      time.Duration
	systemPrompt string
	risk         riskInput
	identity     identityInput
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
	identity := normalizedIdentity(cfg.HumanName, cfg.HumanAlias, cfg.AgentName)
	systemPrompt = buildIdentitySystemPrompt(systemPrompt, identity)

	return &Agent{
		broker:       broker,
		client:       client,
		model:        model,
		timeout:      timeout,
		systemPrompt: systemPrompt,
		risk: riskInput{
			MaxTradeNotional: cfg.MaxTradeNotional,
			MaxDayNotional:   cfg.MaxDayNotional,
			MinGainPct:       cfg.MinGainPct,
		},
		identity:     identity,
		maxWatchlist: 12,
		maxEvents:    defaultMaxPromptEvents,
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

	quotesBySymbol := make(map[string]domain.Quote, len(input.Quotes))
	for _, q := range input.Quotes {
		symbol := strings.ToUpper(strings.TrimSpace(q.Symbol))
		if symbol == "" {
			continue
		}
		quotesBySymbol[symbol] = q
	}
	quotes := make([]quoteInput, 0, len(watchlist))
	quoteErrors := append([]string{}, input.QuoteErrors...)
	for _, symbol := range watchlist {
		q, ok := quotesBySymbol[symbol]
		if !ok {
			var err error
			q, err = a.broker.GetQuote(ctx, symbol)
			if err != nil {
				quoteErrors = append(quoteErrors, fmt.Sprintf("%s: %v", symbol, err))
				continue
			}
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
		Watchlist:   watchlist,
		Account:     input.Snapshot.Account,
		Compliance:  toComplianceInput(input.Compliance),
		Positions:   input.Snapshot.Positions,
		OpenOrders:  openOrdersForPrompt(input.Snapshot.Orders),
		Quotes:      quotes,
		QuoteErrors: quoteErrors,
		Risk:        a.risk,
		Identity:    a.identity,
		RecentEvents: recentEventsForPrompt(
			input.Snapshot.Events,
			a.maxEvents,
		),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal llm input: %w", err)
	}

	content, err := a.completeWithRetry(ctx, string(body))
	if err != nil {
		return nil, err
	}
	return parseIntents(content, watchlist)
}

func (a *Agent) completeWithRetry(ctx context.Context, userPrompt string) (string, error) {
	maxAttempts := 2
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptTimeout := timeoutForAttempt(a.timeout, attempt)
		callCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		content, err := a.client.Complete(callCtx, a.model, a.systemPrompt, userPrompt)
		cancel()
		if err == nil {
			return content, nil
		}
		lastErr = err
		if !shouldRetryLLMError(ctx, err) || attempt == maxAttempts {
			break
		}
	}
	return "", lastErr
}

func timeoutForAttempt(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = 20 * time.Second
	}
	if attempt <= 1 {
		return base
	}
	minRetry := 35 * time.Second
	if base >= minRetry {
		return base
	}
	return minRetry
}

func shouldRetryLLMError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "temporarily unavailable") {
		return true
	}
	return false
}

const defaultSystemPrompt = "You are a conservative US equities trading research assistant. "

const forcedJSONInstruction = "Return strict JSON: {\"intents\":[{\"symbol\":\"AAPL\",\"side\":\"buy|sell\",\"qty\":1.0,\"order_type\":\"market|limit\",\"limit_price\":123.45,\"confidence\":0.0,\"expected_gain_pct\":1.5,\"rationale\":\"...\"}]}. " +
	"Include expected_gain_pct for every intent. " +
	"Avoid dust-sized orders when possible. " +
	"Respect risk.max_trade_notional and risk.max_day_notional from the JSON input. " +
	"Respect risk.min_gain_pct from the JSON input and avoid intents expected below that threshold. " +
	"Respect compliance fields from the JSON input, especially PDT/GFV posture and any drift flags. " +
	"For each intent, size qty so estimated notional stays within risk.max_trade_notional. " +
	"Keep rationale concise and tied to the provided quote and position data." +
	"Only propose watchlist symbols. Keep qty positive. " +
	"Return {\"intents\":[]} when uncertain."

type llmInput struct {
	GeneratedAt  string            `json:"generated_at"`
	Mode         string            `json:"mode"`
	Watchlist    []string          `json:"watchlist"`
	Account      domain.Account    `json:"account"`
	Compliance   *complianceInput  `json:"compliance,omitempty"`
	Positions    []domain.Position `json:"positions"`
	OpenOrders   []orderInput      `json:"open_orders"`
	Quotes       []quoteInput      `json:"quotes"`
	QuoteErrors  []string          `json:"quote_errors"`
	Risk         riskInput         `json:"risk"`
	Identity     identityInput     `json:"identity"`
	RecentEvents []eventInput      `json:"recent_events"`
}

type identityInput struct {
	HumanName  string `json:"human_name"`
	HumanAlias string `json:"human_alias,omitempty"`
	AgentName  string `json:"agent_name"`
}

type riskInput struct {
	MaxTradeNotional float64 `json:"max_trade_notional"`
	MaxDayNotional   float64 `json:"max_day_notional"`
	MinGainPct       float64 `json:"min_gain_pct"`
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
	Time            string `json:"time"`
	Type            string `json:"type"`
	Details         string `json:"details"`
	RejectionReason string `json:"rejection_reason,omitempty"`
}

type complianceInput struct {
	Enabled                 bool    `json:"enabled"`
	AccountType             string  `json:"account_type"`
	AvoidPDT                bool    `json:"avoid_pdt"`
	AvoidGoodFaith          bool    `json:"avoid_gfv"`
	PatternDayTrader        bool    `json:"pattern_day_trader"`
	DayTradeCount           int     `json:"day_trade_count"`
	MaxDayTrades5D          int     `json:"max_day_trades_5d"`
	MinEquityForPDT         float64 `json:"min_equity_for_pdt"`
	Equity                  float64 `json:"equity"`
	LocalUnsettledProceeds  float64 `json:"local_unsettled_proceeds"`
	BrokerUnsettledProceeds float64 `json:"broker_unsettled_proceeds"`
	UnsettledDrift          float64 `json:"unsettled_drift"`
	UnsettledDriftDetected  bool    `json:"unsettled_drift_detected"`
	UnsettledDriftTolerance float64 `json:"unsettled_drift_tolerance"`
	LastReconciledAt        string  `json:"last_reconciled_at,omitempty"`
}

func toComplianceInput(in *domain.ComplianceStatus) *complianceInput {
	if in == nil {
		return nil
	}
	out := &complianceInput{
		Enabled:                 in.Enabled,
		AccountType:             in.AccountType,
		AvoidPDT:                in.AvoidPDT,
		AvoidGoodFaith:          in.AvoidGoodFaith,
		PatternDayTrader:        in.PatternDayTrader,
		DayTradeCount:           in.DayTradeCount,
		MaxDayTrades5D:          in.MaxDayTrades5D,
		MinEquityForPDT:         in.MinEquityForPDT,
		Equity:                  in.Equity,
		LocalUnsettledProceeds:  in.LocalUnsettledProceeds,
		BrokerUnsettledProceeds: in.BrokerUnsettledProceeds,
		UnsettledDrift:          in.UnsettledDrift,
		UnsettledDriftDetected:  in.UnsettledDriftDetected,
		UnsettledDriftTolerance: in.UnsettledDriftTolerance,
	}
	if !in.LastReconciledAt.IsZero() {
		out.LastReconciledAt = in.LastReconciledAt.UTC().Format(time.RFC3339)
	}
	return out
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
	if limit <= 0 || len(events) == 0 {
		return nil
	}

	filtered := make([]domain.Event, 0, limit)
	seen := make(map[string]struct{}, limit)
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if !isRelevantEventForLLM(e) {
			continue
		}
		e.Details = sanitizeEventDetailForLLM(e.Details)
		fingerprint := eventFingerprintForLLM(e)
		if _, ok := seen[fingerprint]; ok {
			continue
		}
		seen[fingerprint] = struct{}{}
		filtered = append(filtered, e)
		if len(filtered) >= limit {
			break
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	// Preserve chronological order after backward scan.
	for left, right := 0, len(filtered)-1; left < right; left, right = left+1, right-1 {
		filtered[left], filtered[right] = filtered[right], filtered[left]
	}
	return toEventInputs(filtered)
}

func toEventInputs(events []domain.Event) []eventInput {
	out := make([]eventInput, 0, len(events))
	for _, e := range events {
		out = append(out, eventInput{
			Time:            e.Time.UTC().Format(time.RFC3339),
			Type:            e.Type,
			Details:         e.Details,
			RejectionReason: sanitizeRejectionReasonForLLM(e.RejectionReason),
		})
	}
	return out
}

func isRelevantEventForLLM(e domain.Event) bool {
	t := strings.ToLower(strings.TrimSpace(e.Type))
	switch t {
	case "order_placed", "order_canceled", "agent_intent_executed", "agent_intent_rejected":
		return true
	case "trade_update":
		d := strings.ToLower(e.Details)
		return strings.Contains(d, "status=filled") ||
			strings.Contains(d, "status=canceled") ||
			strings.Contains(d, "status=cancelled") ||
			strings.Contains(d, "status=rejected")
	default:
		return false
	}
}

func eventFingerprintForLLM(e domain.Event) string {
	return strings.ToLower(strings.TrimSpace(e.Type)) + "|" + e.Details + "|" + strings.TrimSpace(e.RejectionReason)
}

func sanitizeEventDetailForLLM(detail string) string {
	detail = strings.Join(strings.Fields(strings.TrimSpace(detail)), " ")
	if detail == "" {
		return detail
	}
	if len([]rune(detail)) <= maxEventDetailChars {
		return detail
	}
	runes := []rune(detail)
	return string(runes[:maxEventDetailChars]) + "..."
}

func sanitizeRejectionReasonForLLM(reason string) string {
	reason = strings.Join(strings.Fields(strings.TrimSpace(reason)), " ")
	if reason == "" {
		return ""
	}
	if len([]rune(reason)) <= maxRejectionReasonChars {
		return reason
	}
	runes := []rune(reason)
	return string(runes[:maxRejectionReasonChars]) + "..."
}

func normalizedIdentity(humanName, humanAlias, agentName string) identityInput {
	identity := identityInput{
		HumanName:  strings.TrimSpace(humanName),
		HumanAlias: strings.TrimSpace(humanAlias),
		AgentName:  strings.TrimSpace(agentName),
	}
	if identity.HumanName == "" {
		identity.HumanName = "Operator"
	}
	if identity.AgentName == "" {
		identity.AgentName = "Helix"
	}
	return identity
}

func buildIdentitySystemPrompt(base string, identity identityInput) string {
	base = strings.TrimSpace(base)
	identityBlock := fmt.Sprintf(
		"Identity context: You are %s, the execution agent for %s",
		identity.AgentName,
		identity.HumanName,
	)
	if identity.HumanAlias != "" {
		identityBlock += " (" + identity.HumanAlias + ")"
	}
	identityBlock += "."
	if base == "" {
		return identityBlock
	}
	return identityBlock + "\n" + base
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
	Expected   float64  `json:"expected_gain_pct"`
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
		limitPrice := intent.LimitPrice
		switch orderType {
		case domain.OrderTypeMarket:
			limitPrice = nil
		case domain.OrderTypeLimit:
			if intent.LimitPrice == nil || *intent.LimitPrice <= 0 {
				continue
			}
		default:
			orderType = domain.OrderTypeMarket
			limitPrice = nil
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
			Symbol:          symbol,
			Side:            side,
			Qty:             intent.Qty,
			OrderType:       orderType,
			LimitPrice:      limitPrice,
			Confidence:      conf,
			ExpectedGainPct: intent.Expected,
			Rationale:       rationale,
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
	instructions := strings.TrimSpace(systemPrompt)
	if instructions != "" {
		instructions += "\n"
	}
	instructions += forcedJSONInstruction

	response, err := c.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:        shared.ResponsesModel(model),
		Instructions: openai.String(instructions),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String("JSON input:\n" + userPrompt),
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	if response == nil {
		return "", fmt.Errorf("openai response is nil")
	}
	if msg := strings.TrimSpace(response.Error.Message); msg != "" {
		return "", fmt.Errorf("openai response failed: %s", msg)
	}
	content := strings.TrimSpace(response.OutputText())
	if content == "" {
		return "", fmt.Errorf("openai response is empty")
	}
	return content, nil
}
