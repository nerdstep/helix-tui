package strategy

import (
	"context"
	"encoding/json"
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
	"helix-tui/internal/util"
)

const (
	defaultStrategyCopilotSystemPrompt = "You are the strategy copilot for helix-tui. Help the operator refine trading plans, watchlist ideas, and risk-aware tactics."
	forcedStrategyCopilotInstruction   = "Respond in plain text. Be concise, concrete, and advisory-only. Do not claim orders were placed. " +
		"If proposing actionable updates, append a <helix_proposal>...</helix_proposal> JSON block. " +
		"Supported JSON shape: {\"watchlist_proposal\":{\"add\":[\"AAPL\"],\"remove\":[\"TSLA\"],\"rationale\":\"...\"},\"steering_proposal\":{\"risk_profile\":\"balanced\",\"min_confidence\":0.6,\"max_position_notional\":3000,\"horizon\":\"swing\",\"objective\":\"...\",\"preferred_symbols\":[\"AAPL\"],\"excluded_symbols\":[\"AMC\"],\"rationale\":\"...\"}}."
	defaultCopilotMaxMessages     = 40
	defaultCopilotMaxEvents       = 20
	defaultCopilotMaxContentChars = 600
)

var helixProposalBlockPattern = regexp.MustCompile(`(?is)<helix_proposal>\s*(\{.*?\})\s*</helix_proposal>`)

type LLMCopilotConfig struct {
	APIKey       string
	BaseURL      string
	Model        string
	Timeout      time.Duration
	SystemPrompt string
	HumanName    string
	HumanAlias   string
	AgentName    string
}

type strategyCopilotClient interface {
	Complete(ctx context.Context, model, instructions, userPrompt string) (string, error)
}

type LLMCopilot struct {
	client       strategyCopilotClient
	model        string
	timeout      time.Duration
	systemPrompt string
	identity     strategyIdentityInput
	maxMessages  int
	maxEvents    int
}

func NewLLMCopilot(cfg LLMCopilotConfig) (*LLMCopilot, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("strategy copilot api key is required")
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
		systemPrompt = defaultStrategyCopilotSystemPrompt
	}
	identity := normalizedStrategyIdentity(cfg.HumanName, cfg.HumanAlias, cfg.AgentName)
	systemPrompt = buildStrategyIdentitySystemPrompt(systemPrompt, identity)
	return newLLMCopilotWithClient(cfg, &openAIStrategyCopilotClient{
		client: openai.NewClient(buildOpenAIOptions(cfg.APIKey, cfg.BaseURL)...),
	})
}

func newLLMCopilotWithClient(cfg LLMCopilotConfig, client strategyCopilotClient) (*LLMCopilot, error) {
	if client == nil {
		return nil, fmt.Errorf("strategy copilot client is required")
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
		systemPrompt = defaultStrategyCopilotSystemPrompt
	}
	identity := normalizedStrategyIdentity(cfg.HumanName, cfg.HumanAlias, cfg.AgentName)
	systemPrompt = buildStrategyIdentitySystemPrompt(systemPrompt, identity)
	return &LLMCopilot{
		client:       client,
		model:        model,
		timeout:      timeout,
		systemPrompt: systemPrompt,
		identity:     identity,
		maxMessages:  defaultCopilotMaxMessages,
		maxEvents:    defaultCopilotMaxEvents,
	}, nil
}

type strategyCopilotPayload struct {
	GeneratedAt  string                        `json:"generated_at"`
	Identity     strategyIdentityInput         `json:"identity"`
	Watchlist    []string                      `json:"watchlist"`
	Account      domain.Account                `json:"account"`
	Positions    []domain.Position             `json:"positions"`
	OpenOrders   []domain.Order                `json:"open_orders"`
	Quotes       []domain.Quote                `json:"quotes"`
	CurrentPlan  *llmCurrentPlan               `json:"current_plan,omitempty"`
	Messages     []strategyCopilotMessageInput `json:"messages"`
	RecentEvents []strategyCopilotEventInput   `json:"recent_events"`
}

type copilotProposalEnvelope struct {
	WatchlistProposal *copilotWatchlistProposal `json:"watchlist_proposal,omitempty"`
	Watchlist         *copilotWatchlistProposal `json:"watchlist,omitempty"`
	SteeringProposal  *copilotSteeringProposal  `json:"steering_proposal,omitempty"`
	Steering          *copilotSteeringProposal  `json:"steering,omitempty"`
}

type copilotWatchlistProposal struct {
	Add       []string `json:"add"`
	Remove    []string `json:"remove"`
	Rationale string   `json:"rationale"`
}

type copilotSteeringProposal struct {
	RiskProfile         string   `json:"risk_profile"`
	MinConfidence       float64  `json:"min_confidence"`
	MaxPositionNotional float64  `json:"max_position_notional"`
	Horizon             string   `json:"horizon"`
	Objective           string   `json:"objective"`
	PreferredSymbols    []string `json:"preferred_symbols"`
	ExcludedSymbols     []string `json:"excluded_symbols"`
	Rationale           string   `json:"rationale"`
}

type strategyCopilotMessageInput struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at,omitempty"`
}

type strategyCopilotEventInput struct {
	Time    string `json:"time"`
	Type    string `json:"type"`
	Details string `json:"details"`
}

func (c *LLMCopilot) Reply(ctx context.Context, input ChatInput) (ChatReply, error) {
	if c == nil {
		return ChatReply{}, fmt.Errorf("strategy copilot is not initialized")
	}
	payload := strategyCopilotPayload{
		GeneratedAt: input.GeneratedAt.UTC().Format(time.RFC3339),
		Identity:    c.identity,
		Watchlist:   symbols.Normalize(input.Watchlist),
		Account:     input.Snapshot.Account,
		Positions:   append([]domain.Position{}, input.Snapshot.Positions...),
		OpenOrders:  append([]domain.Order{}, input.Snapshot.Orders...),
		Quotes:      append([]domain.Quote{}, input.Quotes...),
		CurrentPlan: toLLMCurrentPlan(input.CurrentPlan),
		Messages:    toStrategyCopilotMessageInputs(input.Messages, c.maxMessages),
		RecentEvents: toStrategyCopilotEventInputs(
			input.RecentEvents,
			c.maxEvents,
		),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatReply{}, fmt.Errorf("marshal strategy copilot payload: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	instructions := c.systemPrompt + "\n" + forcedStrategyCopilotInstruction
	content, err := c.client.Complete(callCtx, c.model, instructions, "JSON input:\n"+string(body))
	if err != nil {
		return ChatReply{}, fmt.Errorf("strategy copilot request failed: %w", err)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return ChatReply{}, fmt.Errorf("strategy copilot response is empty")
	}
	cleaned, proposals := extractCopilotProposals(content)
	if cleaned == "" && len(proposals) > 0 {
		cleaned = "Proposal captured."
	}
	if cleaned == "" {
		cleaned = content
	}
	return ChatReply{
		Content:   cleaned,
		Model:     c.model,
		Proposals: proposals,
	}, nil
}

func toStrategyCopilotMessageInputs(in []ChatMessage, limit int) []strategyCopilotMessageInput {
	if limit <= 0 || len(in) <= limit {
		return buildStrategyCopilotMessageInputs(in)
	}
	return buildStrategyCopilotMessageInputs(in[len(in)-limit:])
}

func buildStrategyCopilotMessageInputs(in []ChatMessage) []strategyCopilotMessageInput {
	out := make([]strategyCopilotMessageInput, 0, len(in))
	for _, msg := range in {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		content := truncateStrategyCopilotText(msg.Content, defaultCopilotMaxContentChars)
		if content == "" {
			continue
		}
		item := strategyCopilotMessageInput{
			Role:    role,
			Content: content,
		}
		if !msg.CreatedAt.IsZero() {
			item.CreatedAt = msg.CreatedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out
}

func toStrategyCopilotEventInputs(in []domain.Event, limit int) []strategyCopilotEventInput {
	if limit <= 0 || len(in) <= limit {
		return buildStrategyCopilotEventInputs(in)
	}
	return buildStrategyCopilotEventInputs(in[len(in)-limit:])
}

func buildStrategyCopilotEventInputs(in []domain.Event) []strategyCopilotEventInput {
	out := make([]strategyCopilotEventInput, 0, len(in))
	for _, event := range in {
		details := truncateStrategyCopilotText(event.Details, 200)
		if details == "" {
			continue
		}
		out = append(out, strategyCopilotEventInput{
			Time:    event.Time.UTC().Format(time.RFC3339),
			Type:    strings.TrimSpace(event.Type),
			Details: details,
		})
	}
	return out
}

func truncateStrategyCopilotText(value string, maxChars int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" || maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	return string(runes[:maxChars]) + "..."
}

func extractCopilotProposals(content string) (string, []CopilotProposal) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", nil
	}
	matches := helixProposalBlockPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content, nil
	}
	proposals := make([]CopilotProposal, 0, len(matches)*2)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		proposals = append(proposals, parseCopilotProposalBlock(match[1])...)
	}
	cleaned := strings.TrimSpace(helixProposalBlockPattern.ReplaceAllString(content, ""))
	return cleaned, proposals
}

func parseCopilotProposalBlock(raw string) []CopilotProposal {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var envelope copilotProposalEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil
	}

	out := make([]CopilotProposal, 0, 2)
	watchlist := envelope.WatchlistProposal
	if watchlist == nil {
		watchlist = envelope.Watchlist
	}
	if watchlist != nil {
		addSymbols := symbols.Normalize(watchlist.Add)
		removeSymbols := symbols.Normalize(watchlist.Remove)
		if len(addSymbols) > 0 && len(removeSymbols) > 0 {
			addSet := make(map[string]struct{}, len(addSymbols))
			for _, symbol := range addSymbols {
				addSet[symbol] = struct{}{}
			}
			filtered := make([]string, 0, len(removeSymbols))
			for _, symbol := range removeSymbols {
				if _, exists := addSet[symbol]; exists {
					continue
				}
				filtered = append(filtered, symbol)
			}
			removeSymbols = filtered
		}
		if len(addSymbols) > 0 || len(removeSymbols) > 0 {
			out = append(out, CopilotProposal{
				Kind:          CopilotProposalKindWatchlist,
				Rationale:     strings.TrimSpace(watchlist.Rationale),
				AddSymbols:    addSymbols,
				RemoveSymbols: removeSymbols,
			})
		}
	}

	steering := envelope.SteeringProposal
	if steering == nil {
		steering = envelope.Steering
	}
	if steering != nil {
		preferredSymbols := symbols.Normalize(steering.PreferredSymbols)
		excludedSymbols := symbols.Normalize(steering.ExcludedSymbols)
		if len(preferredSymbols) > 0 && len(excludedSymbols) > 0 {
			preferredSet := make(map[string]struct{}, len(preferredSymbols))
			for _, symbol := range preferredSymbols {
				preferredSet[symbol] = struct{}{}
			}
			filtered := make([]string, 0, len(excludedSymbols))
			for _, symbol := range excludedSymbols {
				if _, exists := preferredSet[symbol]; exists {
					continue
				}
				filtered = append(filtered, symbol)
			}
			excludedSymbols = filtered
		}
		riskProfile := strings.ToLower(strings.TrimSpace(steering.RiskProfile))
		minConfidence := util.Clamp01(steering.MinConfidence)
		maxPositionNotional := util.MaxFloat(steering.MaxPositionNotional, 0)
		horizon := strings.ToLower(strings.TrimSpace(steering.Horizon))
		objective := strings.TrimSpace(steering.Objective)
		if riskProfile != "" ||
			minConfidence > 0 ||
			maxPositionNotional > 0 ||
			horizon != "" ||
			objective != "" ||
			len(preferredSymbols) > 0 ||
			len(excludedSymbols) > 0 {
			out = append(out, CopilotProposal{
				Kind:                CopilotProposalKindSteering,
				Rationale:           strings.TrimSpace(steering.Rationale),
				RiskProfile:         riskProfile,
				MinConfidence:       minConfidence,
				MaxPositionNotional: maxPositionNotional,
				Horizon:             horizon,
				Objective:           objective,
				PreferredSymbols:    preferredSymbols,
				ExcludedSymbols:     excludedSymbols,
			})
		}
	}
	return out
}

func buildOpenAIOptions(apiKey string, baseURL string) []option.RequestOption {
	opts := []option.RequestOption{option.WithAPIKey(strings.TrimSpace(apiKey))}
	if trimmed := strings.TrimSpace(baseURL); trimmed != "" {
		opts = append(opts, option.WithBaseURL(trimmed))
	}
	return opts
}

type openAIStrategyCopilotClient struct {
	client openai.Client
}

func (c *openAIStrategyCopilotClient) Complete(ctx context.Context, model, instructions, userPrompt string) (string, error) {
	resp, err := c.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:        shared.ResponsesModel(strings.TrimSpace(model)),
		Instructions: openai.String(strings.TrimSpace(instructions)),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(strings.TrimSpace(userPrompt)),
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfText: &shared.ResponseFormatTextParam{},
			},
		},
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("openai response is nil")
	}
	if msg := strings.TrimSpace(resp.Error.Message); msg != "" {
		return "", fmt.Errorf("openai response failed: %s", msg)
	}
	content := strings.TrimSpace(resp.OutputText())
	if content == "" {
		return "", fmt.Errorf("openai response is empty")
	}
	return content, nil
}
