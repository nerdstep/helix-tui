package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"helix-tui/internal/symbols"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const strategySteeringSingletonID uint = 1

type StrategySteeringState struct {
	ID                  uint
	Version             uint64
	Source              string
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

type StrategySteeringStateInput struct {
	Source              string
	RiskProfile         string
	MinConfidence       float64
	MaxPositionNotional float64
	Horizon             string
	Objective           string
	PreferredSymbols    []string
	ExcludedSymbols     []string
}

type strategySteeringStateRecord struct {
	ID                  uint    `gorm:"primaryKey"`
	Version             uint64  `gorm:"not null;default:1"`
	Source              string  `gorm:"size:64;not null;default:system"`
	RiskProfile         string  `gorm:"size:32"`
	MinConfidence       float64 `gorm:"not null;default:0"`
	MaxPositionNotional float64 `gorm:"not null;default:0"`
	Horizon             string  `gorm:"size:64"`
	Objective           string  `gorm:"type:text"`
	Hash                string  `gorm:"size:64;index;not null;default:''"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (strategySteeringStateRecord) TableName() string {
	return "strategy_steering_state"
}

type strategySteeringSymbolRecord struct {
	ID         uint   `gorm:"primaryKey"`
	SteeringID uint   `gorm:"index;not null"`
	Symbol     string `gorm:"size:32;index;not null"`
	Kind       string `gorm:"size:16;index;not null"` // preferred | excluded
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (strategySteeringSymbolRecord) TableName() string {
	return "strategy_steering_symbols"
}

func (r *StrategyRepository) GetSteeringState() (*StrategySteeringState, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	var stateRecord strategySteeringStateRecord
	res := r.db.Where("id = ?", strategySteeringSingletonID).Limit(1).Find(&stateRecord)
	if res.Error != nil {
		return nil, fmt.Errorf("query strategy steering state: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	symbolsByKind, err := r.loadSteeringSymbols(stateRecord.ID)
	if err != nil {
		return nil, err
	}
	return &StrategySteeringState{
		ID:                  stateRecord.ID,
		Version:             stateRecord.Version,
		Source:              strings.TrimSpace(stateRecord.Source),
		RiskProfile:         strings.TrimSpace(stateRecord.RiskProfile),
		MinConfidence:       stateRecord.MinConfidence,
		MaxPositionNotional: stateRecord.MaxPositionNotional,
		Horizon:             strings.TrimSpace(stateRecord.Horizon),
		Objective:           strings.TrimSpace(stateRecord.Objective),
		PreferredSymbols:    symbolsByKind["preferred"],
		ExcludedSymbols:     symbolsByKind["excluded"],
		Hash:                strings.TrimSpace(stateRecord.Hash),
		CreatedAt:           stateRecord.CreatedAt.UTC(),
		UpdatedAt:           stateRecord.UpdatedAt.UTC(),
	}, nil
}

func (r *StrategyRepository) UpsertSteeringState(input StrategySteeringStateInput) (StrategySteeringState, error) {
	if r == nil || r.db == nil {
		return StrategySteeringState{}, fmt.Errorf("strategy repository is not initialized")
	}
	normalized, hash := normalizeSteeringInput(input)
	var out StrategySteeringState
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var current strategySteeringStateRecord
		res := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", strategySteeringSingletonID).
			Limit(1).
			Find(&current)
		if res.Error != nil {
			return fmt.Errorf("query strategy steering state: %w", res.Error)
		}

		version := uint64(1)
		createdAt := time.Now().UTC()
		if res.RowsAffected > 0 {
			createdAt = current.CreatedAt
			version = current.Version
			if strings.TrimSpace(current.Hash) != hash {
				version = current.Version + 1
			}
		}

		record := strategySteeringStateRecord{
			ID:                  strategySteeringSingletonID,
			Version:             version,
			Source:              normalized.Source,
			RiskProfile:         normalized.RiskProfile,
			MinConfidence:       normalized.MinConfidence,
			MaxPositionNotional: normalized.MaxPositionNotional,
			Horizon:             normalized.Horizon,
			Objective:           normalized.Objective,
			Hash:                hash,
			CreatedAt:           createdAt,
		}
		if err := tx.Save(&record).Error; err != nil {
			return fmt.Errorf("upsert strategy steering state: %w", err)
		}
		if err := tx.Where("steering_id = ?", record.ID).Delete(&strategySteeringSymbolRecord{}).Error; err != nil {
			return fmt.Errorf("delete strategy steering symbols: %w", err)
		}
		symbolRows := toSteeringSymbolRecords(record.ID, normalized.PreferredSymbols, normalized.ExcludedSymbols)
		if len(symbolRows) > 0 {
			if err := tx.Create(&symbolRows).Error; err != nil {
				return fmt.Errorf("insert strategy steering symbols: %w", err)
			}
		}

		out = StrategySteeringState{
			ID:                  record.ID,
			Version:             record.Version,
			Source:              record.Source,
			RiskProfile:         record.RiskProfile,
			MinConfidence:       record.MinConfidence,
			MaxPositionNotional: record.MaxPositionNotional,
			Horizon:             record.Horizon,
			Objective:           record.Objective,
			PreferredSymbols:    append([]string{}, normalized.PreferredSymbols...),
			ExcludedSymbols:     append([]string{}, normalized.ExcludedSymbols...),
			Hash:                record.Hash,
			CreatedAt:           record.CreatedAt.UTC(),
			UpdatedAt:           record.UpdatedAt.UTC(),
		}
		return nil
	})
	if err != nil {
		return StrategySteeringState{}, err
	}
	return out, nil
}

func (r *StrategyRepository) loadSteeringSymbols(steeringID uint) (map[string][]string, error) {
	out := map[string][]string{
		"preferred": {},
		"excluded":  {},
	}
	var rows []strategySteeringSymbolRecord
	if err := r.db.
		Where("steering_id = ?", steeringID).
		Order("kind asc, symbol asc").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query strategy steering symbols: %w", err)
	}
	for _, row := range rows {
		kind := strings.ToLower(strings.TrimSpace(row.Kind))
		if kind != "preferred" && kind != "excluded" {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(row.Symbol))
		if symbol == "" {
			continue
		}
		out[kind] = append(out[kind], symbol)
	}
	return out, nil
}

func toSteeringSymbolRecords(steeringID uint, preferred, excluded []string) []strategySteeringSymbolRecord {
	rows := make([]strategySteeringSymbolRecord, 0, len(preferred)+len(excluded))
	for _, symbol := range preferred {
		rows = append(rows, strategySteeringSymbolRecord{
			SteeringID: steeringID,
			Symbol:     symbol,
			Kind:       "preferred",
		})
	}
	for _, symbol := range excluded {
		rows = append(rows, strategySteeringSymbolRecord{
			SteeringID: steeringID,
			Symbol:     symbol,
			Kind:       "excluded",
		})
	}
	return rows
}

func normalizeSteeringInput(input StrategySteeringStateInput) (StrategySteeringStateInput, string) {
	normalized := StrategySteeringStateInput{
		Source:              normalizeSteeringSource(input.Source),
		RiskProfile:         strings.ToLower(strings.TrimSpace(input.RiskProfile)),
		MinConfidence:       clamp01(input.MinConfidence),
		MaxPositionNotional: maxFloat(input.MaxPositionNotional, 0),
		Horizon:             strings.ToLower(strings.TrimSpace(input.Horizon)),
		Objective:           strings.TrimSpace(input.Objective),
		PreferredSymbols:    normalizeSymbolsForSteering(input.PreferredSymbols),
		ExcludedSymbols:     normalizeSymbolsForSteering(input.ExcludedSymbols),
	}
	if len(normalized.PreferredSymbols) > 0 && len(normalized.ExcludedSymbols) > 0 {
		preferredSet := make(map[string]struct{}, len(normalized.PreferredSymbols))
		for _, symbol := range normalized.PreferredSymbols {
			preferredSet[symbol] = struct{}{}
		}
		filtered := make([]string, 0, len(normalized.ExcludedSymbols))
		for _, symbol := range normalized.ExcludedSymbols {
			if _, exists := preferredSet[symbol]; exists {
				continue
			}
			filtered = append(filtered, symbol)
		}
		normalized.ExcludedSymbols = filtered
	}
	return normalized, hashSteeringInput(normalized)
}

func normalizeSteeringSource(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "system"
	}
	return value
}

func normalizeSymbolsForSteering(values []string) []string {
	normalized := symbols.Normalize(values)
	sort.Strings(normalized)
	return normalized
}

func hashSteeringInput(input StrategySteeringStateInput) string {
	body, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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
