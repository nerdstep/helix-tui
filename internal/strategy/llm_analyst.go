package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

const defaultStrategySystemPrompt = "You are a senior US equities strategy analyst for helix-tui. Build a concise, risk-aware strategy plan from the provided JSON context."

const forcedStrategyJSONInstruction = "Return strict JSON: {\"no_change\":false,\"summary\":\"...\",\"confidence\":0.0,\"recommendations\":[{\"symbol\":\"AAPL\",\"bias\":\"buy|sell|hold\",\"confidence\":0.0,\"entry_min\":0,\"entry_max\":0,\"target_price\":0,\"stop_price\":0,\"max_notional\":0,\"thesis\":\"...\",\"invalidation\":\"...\",\"priority\":1}]}. " +
	"If current_plan is still valid with no material updates, set no_change=true and return an empty recommendations array. " +
	"Prefer at most the requested max_recommendations and keep recommendations actionable."

type LLMAnalystConfig struct {
	APIKey             string
	BaseURL            string
	Model              string
	Timeout            time.Duration
	SystemPrompt       string
	MaxRecommendations int
	HumanName          string
	HumanAlias         string
	AgentName          string
}

type LLMAnalyst struct {
	client             openai.Client
	model              string
	timeout            time.Duration
	systemPrompt       string
	maxRecommendations int
	identity           strategyIdentityInput
}

func NewLLMAnalyst(cfg LLMAnalystConfig) (*LLMAnalyst, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("strategy llm api key is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-5"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultStrategySystemPrompt
	}
	identity := normalizedStrategyIdentity(cfg.HumanName, cfg.HumanAlias, cfg.AgentName)
	systemPrompt = buildStrategyIdentitySystemPrompt(systemPrompt, identity)
	maxRecommendations := cfg.MaxRecommendations
	if maxRecommendations <= 0 {
		maxRecommendations = 8
	}

	opts := []option.RequestOption{option.WithAPIKey(strings.TrimSpace(cfg.APIKey))}
	if base := strings.TrimSpace(cfg.BaseURL); base != "" {
		opts = append(opts, option.WithBaseURL(base))
	}
	return &LLMAnalyst{
		client:             openai.NewClient(opts...),
		model:              model,
		timeout:            timeout,
		systemPrompt:       systemPrompt,
		maxRecommendations: maxRecommendations,
		identity:           identity,
	}, nil
}

type llmStrategyInput struct {
	GeneratedAt        string                `json:"generated_at"`
	MaxRecommendations int                   `json:"max_recommendations"`
	Watchlist          []string              `json:"watchlist"`
	CurrentPlan        *llmCurrentPlan       `json:"current_plan,omitempty"`
	Account            domain.Account        `json:"account"`
	Positions          []domain.Position     `json:"positions"`
	OpenOrders         []domain.Order        `json:"open_orders"`
	Quotes             []domain.Quote        `json:"quotes"`
	Identity           strategyIdentityInput `json:"identity"`
	RecentEvents       []domain.Event        `json:"recent_events"`
}

type strategyIdentityInput struct {
	HumanName  string `json:"human_name"`
	HumanAlias string `json:"human_alias,omitempty"`
	AgentName  string `json:"agent_name"`
}

type llmStrategyOutput struct {
	NoChange        bool                        `json:"no_change"`
	Summary         string                      `json:"summary"`
	Confidence      float64                     `json:"confidence"`
	Recommendations []llmStrategyRecommendation `json:"recommendations"`
}

type llmStrategyRecommendation struct {
	Symbol       string  `json:"symbol"`
	Bias         string  `json:"bias"`
	Confidence   float64 `json:"confidence"`
	EntryMin     float64 `json:"entry_min"`
	EntryMax     float64 `json:"entry_max"`
	TargetPrice  float64 `json:"target_price"`
	StopPrice    float64 `json:"stop_price"`
	MaxNotional  float64 `json:"max_notional"`
	Thesis       string  `json:"thesis"`
	Invalidation string  `json:"invalidation"`
	Priority     int     `json:"priority"`
}

type llmCurrentPlan struct {
	ID              uint                        `json:"id"`
	GeneratedAt     string                      `json:"generated_at"`
	Status          string                      `json:"status"`
	Summary         string                      `json:"summary"`
	Confidence      float64                     `json:"confidence"`
	Recommendations []llmStrategyRecommendation `json:"recommendations"`
}

func (a *LLMAnalyst) BuildPlan(ctx context.Context, input Input) (Plan, error) {
	if a == nil {
		return Plan{}, fmt.Errorf("strategy analyst is not initialized")
	}
	payload := llmStrategyInput{
		GeneratedAt:        input.GeneratedAt.UTC().Format(time.RFC3339),
		MaxRecommendations: minInt(a.maxRecommendations, maxInt(1, input.MaxRecommendations)),
		Watchlist:          symbols.Normalize(input.Watchlist),
		CurrentPlan:        toLLMCurrentPlan(input.CurrentPlan),
		Account:            input.Snapshot.Account,
		Positions:          input.Snapshot.Positions,
		OpenOrders:         input.Snapshot.Orders,
		Quotes:             input.Quotes,
		Identity:           a.identity,
		RecentEvents:       trimRecentEvents(input.RecentEvents, 24),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Plan{}, fmt.Errorf("marshal strategy payload: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.Chat.Completions.New(callCtx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(a.model),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
		},
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(a.systemPrompt),
			openai.SystemMessage(forcedStrategyJSONInstruction),
			openai.UserMessage("JSON input:\n" + string(body)),
		},
	})
	if err != nil {
		return Plan{}, fmt.Errorf("openai request failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Plan{}, fmt.Errorf("openai response has no choices")
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return Plan{}, fmt.Errorf("openai response is empty")
	}

	var parsed llmStrategyOutput
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return Plan{}, fmt.Errorf("parse strategy llm output: %w", err)
	}

	out := Plan{
		NoChange:   parsed.NoChange,
		Summary:    strings.TrimSpace(parsed.Summary),
		Confidence: clamp01(parsed.Confidence),
	}
	if out.NoChange {
		out.Recommendations = nil
		return out, nil
	}
	allowed := map[string]struct{}{}
	for _, symbol := range payload.Watchlist {
		allowed[symbol] = struct{}{}
	}
	recs := make([]Recommendation, 0, len(parsed.Recommendations))
	for i, rec := range parsed.Recommendations {
		normalized := symbols.Normalize([]string{rec.Symbol})
		if len(normalized) == 0 {
			continue
		}
		symbol := normalized[0]
		if len(allowed) > 0 {
			if _, ok := allowed[symbol]; !ok {
				continue
			}
		}
		bias := strings.ToLower(strings.TrimSpace(rec.Bias))
		if bias == "" {
			bias = "hold"
		}
		priority := rec.Priority
		if priority <= 0 {
			priority = i + 1
		}
		recs = append(recs, Recommendation{
			Symbol:       symbol,
			Bias:         bias,
			Confidence:   clamp01(rec.Confidence),
			EntryMin:     rec.EntryMin,
			EntryMax:     rec.EntryMax,
			TargetPrice:  rec.TargetPrice,
			StopPrice:    rec.StopPrice,
			MaxNotional:  rec.MaxNotional,
			Thesis:       strings.TrimSpace(rec.Thesis),
			Invalidation: strings.TrimSpace(rec.Invalidation),
			Priority:     priority,
		})
		if len(recs) >= payload.MaxRecommendations {
			break
		}
	}
	out.Recommendations = recs
	return out, nil
}

func toLLMCurrentPlan(current *CurrentPlan) *llmCurrentPlan {
	if current == nil {
		return nil
	}
	recs := make([]llmStrategyRecommendation, 0, len(current.Recommendations))
	for _, rec := range current.Recommendations {
		recs = append(recs, llmStrategyRecommendation{
			Symbol:       rec.Symbol,
			Bias:         rec.Bias,
			Confidence:   rec.Confidence,
			EntryMin:     rec.EntryMin,
			EntryMax:     rec.EntryMax,
			TargetPrice:  rec.TargetPrice,
			StopPrice:    rec.StopPrice,
			MaxNotional:  rec.MaxNotional,
			Thesis:       rec.Thesis,
			Invalidation: rec.Invalidation,
			Priority:     rec.Priority,
		})
	}
	return &llmCurrentPlan{
		ID:              current.ID,
		GeneratedAt:     current.GeneratedAt.UTC().Format(time.RFC3339),
		Status:          strings.TrimSpace(current.Status),
		Summary:         strings.TrimSpace(current.Summary),
		Confidence:      current.Confidence,
		Recommendations: recs,
	}
}

func trimRecentEvents(events []domain.Event, limit int) []domain.Event {
	if limit <= 0 || len(events) <= limit {
		return events
	}
	return events[len(events)-limit:]
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func normalizedStrategyIdentity(humanName, humanAlias, agentName string) strategyIdentityInput {
	identity := strategyIdentityInput{
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

func buildStrategyIdentitySystemPrompt(base string, identity strategyIdentityInput) string {
	base = strings.TrimSpace(base)
	identityBlock := fmt.Sprintf(
		"Identity context: You are %s, the strategy analyst for %s",
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
