package storage

import (
	"fmt"
	"strings"
	"time"

	"helix-tui/internal/symbols"
	"helix-tui/internal/util"
)

const defaultRecentStrategyProposalLimit = 30

type StrategyProposalKind string

const (
	StrategyProposalKindWatchlist StrategyProposalKind = "watchlist"
	StrategyProposalKindSteering  StrategyProposalKind = "steering"
)

type StrategyProposalStatus string

const (
	StrategyProposalStatusPending  StrategyProposalStatus = "pending"
	StrategyProposalStatusApplied  StrategyProposalStatus = "applied"
	StrategyProposalStatusRejected StrategyProposalStatus = "rejected"
)

type StrategyProposal struct {
	ID                  uint
	ThreadID            uint
	Source              string
	Kind                StrategyProposalKind
	Status              StrategyProposalStatus
	Rationale           string
	AddSymbols          []string
	RemoveSymbols       []string
	RiskProfile         string
	MinConfidence       float64
	MaxPositionNotional float64
	Horizon             string
	Objective           string
	PreferredSymbols    []string
	ExcludedSymbols     []string
	Hash                string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type StrategyProposalInput struct {
	ThreadID            uint
	Source              string
	Kind                StrategyProposalKind
	Rationale           string
	AddSymbols          []string
	RemoveSymbols       []string
	RiskProfile         string
	MinConfidence       float64
	MaxPositionNotional float64
	Horizon             string
	Objective           string
	PreferredSymbols    []string
	ExcludedSymbols     []string
}

type strategyProposalRecord struct {
	ID                   uint    `gorm:"primaryKey"`
	ThreadID             uint    `gorm:"index;not null"`
	Source               string  `gorm:"size:64;index;not null;default:copilot"`
	Kind                 string  `gorm:"size:24;index;not null"`
	Status               string  `gorm:"size:24;index;not null;default:pending"`
	Rationale            string  `gorm:"type:text"`
	AddSymbolsJSON       string  `gorm:"type:text"`
	RemoveSymbolsJSON    string  `gorm:"type:text"`
	RiskProfile          string  `gorm:"size:32"`
	MinConfidence        float64 `gorm:"not null;default:0"`
	MaxPositionNotional  float64 `gorm:"not null;default:0"`
	Horizon              string  `gorm:"size:64"`
	Objective            string  `gorm:"type:text"`
	PreferredSymbolsJSON string  `gorm:"type:text"`
	ExcludedSymbolsJSON  string  `gorm:"type:text"`
	Hash                 string  `gorm:"size:64;index;not null;default:''"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (strategyProposalRecord) TableName() string {
	return "strategy_proposals"
}

func (r *StrategyRepository) CreateProposal(input StrategyProposalInput) (StrategyProposal, error) {
	if r == nil || r.db == nil {
		return StrategyProposal{}, fmt.Errorf("strategy repository is not initialized")
	}
	normalized, err := normalizeStrategyProposalInput(input)
	if err != nil {
		return StrategyProposal{}, err
	}
	record, err := toStrategyProposalRecord(normalized)
	if err != nil {
		return StrategyProposal{}, err
	}
	record.Status = string(StrategyProposalStatusPending)
	if err := r.db.Create(&record).Error; err != nil {
		return StrategyProposal{}, fmt.Errorf("insert strategy proposal: %w", err)
	}
	return fromStrategyProposalRecord(record)
}

func (r *StrategyRepository) GetProposal(proposalID uint) (*StrategyProposal, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	if proposalID == 0 {
		return nil, fmt.Errorf("strategy proposal id is required")
	}
	var record strategyProposalRecord
	result := r.db.Where("id = ?", proposalID).Limit(1).Find(&record)
	if result.Error != nil {
		return nil, fmt.Errorf("query strategy proposal: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	out, err := fromStrategyProposalRecord(record)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *StrategyRepository) ListProposals(limit int, statuses ...StrategyProposalStatus) ([]StrategyProposal, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	if limit <= 0 {
		limit = defaultRecentStrategyProposalLimit
	}
	query := r.db.Order("id desc").Limit(limit)
	if len(statuses) > 0 {
		normalized := make([]string, 0, len(statuses))
		for _, status := range statuses {
			value, ok := normalizedStrategyProposalStatus(status)
			if !ok {
				return nil, fmt.Errorf("invalid strategy proposal status %q", status)
			}
			normalized = append(normalized, value)
		}
		query = query.Where("status IN ?", normalized)
	}
	var records []strategyProposalRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query strategy proposals: %w", err)
	}
	out := make([]StrategyProposal, 0, len(records))
	for _, record := range records {
		proposal, err := fromStrategyProposalRecord(record)
		if err != nil {
			return nil, err
		}
		out = append(out, proposal)
	}
	return out, nil
}

func (r *StrategyRepository) SetProposalStatus(proposalID uint, status StrategyProposalStatus) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("strategy repository is not initialized")
	}
	if proposalID == 0 {
		return fmt.Errorf("strategy proposal id is required")
	}
	statusValue, ok := normalizedStrategyProposalStatus(status)
	if !ok {
		return fmt.Errorf("invalid strategy proposal status %q", status)
	}
	result := r.db.Model(&strategyProposalRecord{}).
		Where("id = ?", proposalID).
		Update("status", statusValue)
	if result.Error != nil {
		return fmt.Errorf("update strategy proposal status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("strategy proposal %d not found", proposalID)
	}
	return nil
}

func normalizeStrategyProposalInput(input StrategyProposalInput) (StrategyProposalInput, error) {
	out := StrategyProposalInput{
		ThreadID:            input.ThreadID,
		Source:              normalizeStrategyProposalSource(input.Source),
		Kind:                normalizeStrategyProposalKind(input.Kind),
		Rationale:           strings.TrimSpace(input.Rationale),
		AddSymbols:          symbols.NormalizeSorted(input.AddSymbols),
		RemoveSymbols:       symbols.NormalizeSorted(input.RemoveSymbols),
		RiskProfile:         strings.ToLower(strings.TrimSpace(input.RiskProfile)),
		MinConfidence:       util.Clamp01(input.MinConfidence),
		MaxPositionNotional: util.MaxFloat(input.MaxPositionNotional, 0),
		Horizon:             strings.ToLower(strings.TrimSpace(input.Horizon)),
		Objective:           strings.TrimSpace(input.Objective),
		PreferredSymbols:    symbols.NormalizeSorted(input.PreferredSymbols),
		ExcludedSymbols:     symbols.NormalizeSorted(input.ExcludedSymbols),
	}
	if out.ThreadID == 0 {
		return StrategyProposalInput{}, fmt.Errorf("strategy proposal thread id is required")
	}
	if out.Kind == "" {
		return StrategyProposalInput{}, fmt.Errorf("strategy proposal kind is required")
	}
	switch out.Kind {
	case StrategyProposalKindWatchlist:
		out.AddSymbols = util.DedupeSortedStrings(out.AddSymbols)
		out.RemoveSymbols = util.DedupeSortedStrings(out.RemoveSymbols)
		out.RemoveSymbols = util.RemoveOverlappingStrings(out.AddSymbols, out.RemoveSymbols)
		if len(out.AddSymbols) == 0 && len(out.RemoveSymbols) == 0 {
			return StrategyProposalInput{}, fmt.Errorf("watchlist proposal requires add and/or remove symbols")
		}
	case StrategyProposalKindSteering:
		out.PreferredSymbols = util.DedupeSortedStrings(out.PreferredSymbols)
		out.ExcludedSymbols = util.DedupeSortedStrings(out.ExcludedSymbols)
		out.ExcludedSymbols = util.RemoveOverlappingStrings(out.PreferredSymbols, out.ExcludedSymbols)
		if out.RiskProfile == "" &&
			out.MinConfidence <= 0 &&
			out.MaxPositionNotional <= 0 &&
			out.Horizon == "" &&
			out.Objective == "" &&
			len(out.PreferredSymbols) == 0 &&
			len(out.ExcludedSymbols) == 0 {
			return StrategyProposalInput{}, fmt.Errorf("steering proposal has no actionable fields")
		}
	default:
		return StrategyProposalInput{}, fmt.Errorf("unsupported strategy proposal kind %q", out.Kind)
	}
	return out, nil
}

func toStrategyProposalRecord(input StrategyProposalInput) (strategyProposalRecord, error) {
	addJSON, err := util.EncodeStringListJSON(input.AddSymbols)
	if err != nil {
		return strategyProposalRecord{}, fmt.Errorf("encode strategy proposal add symbols: %w", err)
	}
	removeJSON, err := util.EncodeStringListJSON(input.RemoveSymbols)
	if err != nil {
		return strategyProposalRecord{}, fmt.Errorf("encode strategy proposal remove symbols: %w", err)
	}
	preferredJSON, err := util.EncodeStringListJSON(input.PreferredSymbols)
	if err != nil {
		return strategyProposalRecord{}, fmt.Errorf("encode strategy proposal preferred symbols: %w", err)
	}
	excludedJSON, err := util.EncodeStringListJSON(input.ExcludedSymbols)
	if err != nil {
		return strategyProposalRecord{}, fmt.Errorf("encode strategy proposal excluded symbols: %w", err)
	}
	hash := hashStrategyProposalInput(input)
	return strategyProposalRecord{
		ThreadID:             input.ThreadID,
		Source:               input.Source,
		Kind:                 string(input.Kind),
		Status:               string(StrategyProposalStatusPending),
		Rationale:            input.Rationale,
		AddSymbolsJSON:       addJSON,
		RemoveSymbolsJSON:    removeJSON,
		RiskProfile:          input.RiskProfile,
		MinConfidence:        input.MinConfidence,
		MaxPositionNotional:  input.MaxPositionNotional,
		Horizon:              input.Horizon,
		Objective:            input.Objective,
		PreferredSymbolsJSON: preferredJSON,
		ExcludedSymbolsJSON:  excludedJSON,
		Hash:                 hash,
	}, nil
}

func fromStrategyProposalRecord(record strategyProposalRecord) (StrategyProposal, error) {
	addSymbols, err := decodeSymbolListJSON(record.AddSymbolsJSON)
	if err != nil {
		return StrategyProposal{}, fmt.Errorf("decode strategy proposal add symbols: %w", err)
	}
	removeSymbols, err := decodeSymbolListJSON(record.RemoveSymbolsJSON)
	if err != nil {
		return StrategyProposal{}, fmt.Errorf("decode strategy proposal remove symbols: %w", err)
	}
	preferredSymbols, err := decodeSymbolListJSON(record.PreferredSymbolsJSON)
	if err != nil {
		return StrategyProposal{}, fmt.Errorf("decode strategy proposal preferred symbols: %w", err)
	}
	excludedSymbols, err := decodeSymbolListJSON(record.ExcludedSymbolsJSON)
	if err != nil {
		return StrategyProposal{}, fmt.Errorf("decode strategy proposal excluded symbols: %w", err)
	}
	kind := normalizeStrategyProposalKind(StrategyProposalKind(record.Kind))
	if kind == "" {
		return StrategyProposal{}, fmt.Errorf("invalid strategy proposal kind %q", record.Kind)
	}
	status, ok := normalizedStrategyProposalStatus(StrategyProposalStatus(record.Status))
	if !ok {
		return StrategyProposal{}, fmt.Errorf("invalid strategy proposal status %q", record.Status)
	}
	return StrategyProposal{
		ID:                  record.ID,
		ThreadID:            record.ThreadID,
		Source:              normalizeStrategyProposalSource(record.Source),
		Kind:                kind,
		Status:              StrategyProposalStatus(status),
		Rationale:           strings.TrimSpace(record.Rationale),
		AddSymbols:          addSymbols,
		RemoveSymbols:       removeSymbols,
		RiskProfile:         strings.ToLower(strings.TrimSpace(record.RiskProfile)),
		MinConfidence:       util.Clamp01(record.MinConfidence),
		MaxPositionNotional: util.MaxFloat(record.MaxPositionNotional, 0),
		Horizon:             strings.ToLower(strings.TrimSpace(record.Horizon)),
		Objective:           strings.TrimSpace(record.Objective),
		PreferredSymbols:    preferredSymbols,
		ExcludedSymbols:     excludedSymbols,
		Hash:                strings.TrimSpace(record.Hash),
		CreatedAt:           record.CreatedAt.UTC(),
		UpdatedAt:           record.UpdatedAt.UTC(),
	}, nil
}

func normalizeStrategyProposalKind(kind StrategyProposalKind) StrategyProposalKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case string(StrategyProposalKindWatchlist):
		return StrategyProposalKindWatchlist
	case string(StrategyProposalKindSteering):
		return StrategyProposalKindSteering
	default:
		return ""
	}
}

func normalizeStrategyProposalSource(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "copilot"
	}
	return value
}

func normalizedStrategyProposalStatus(status StrategyProposalStatus) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(StrategyProposalStatusPending):
		return string(StrategyProposalStatusPending), true
	case string(StrategyProposalStatusApplied):
		return string(StrategyProposalStatusApplied), true
	case string(StrategyProposalStatusRejected):
		return string(StrategyProposalStatusRejected), true
	default:
		return "", false
	}
}

func decodeSymbolListJSON(raw string) ([]string, error) {
	values, err := util.DecodeStringListJSON(raw)
	if err != nil {
		return nil, err
	}
	return symbols.NormalizeSorted(values), nil
}

func hashStrategyProposalInput(input StrategyProposalInput) string {
	payload := struct {
		Kind                StrategyProposalKind `json:"kind"`
		Rationale           string               `json:"rationale"`
		AddSymbols          []string             `json:"add_symbols,omitempty"`
		RemoveSymbols       []string             `json:"remove_symbols,omitempty"`
		RiskProfile         string               `json:"risk_profile,omitempty"`
		MinConfidence       float64              `json:"min_confidence,omitempty"`
		MaxPositionNotional float64              `json:"max_position_notional,omitempty"`
		Horizon             string               `json:"horizon,omitempty"`
		Objective           string               `json:"objective,omitempty"`
		PreferredSymbols    []string             `json:"preferred_symbols,omitempty"`
		ExcludedSymbols     []string             `json:"excluded_symbols,omitempty"`
	}{
		Kind:                input.Kind,
		Rationale:           strings.TrimSpace(input.Rationale),
		AddSymbols:          util.DedupeSortedStrings(input.AddSymbols),
		RemoveSymbols:       util.DedupeSortedStrings(input.RemoveSymbols),
		RiskProfile:         strings.TrimSpace(input.RiskProfile),
		MinConfidence:       util.Clamp01(input.MinConfidence),
		MaxPositionNotional: util.MaxFloat(input.MaxPositionNotional, 0),
		Horizon:             strings.TrimSpace(input.Horizon),
		Objective:           strings.TrimSpace(input.Objective),
		PreferredSymbols:    util.DedupeSortedStrings(input.PreferredSymbols),
		ExcludedSymbols:     util.DedupeSortedStrings(input.ExcludedSymbols),
	}
	return util.HashJSONSHA256HexOrEmpty(payload)
}
