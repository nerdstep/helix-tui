package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const defaultRecentStrategyPlanLimit = 20

type StrategyPlanStatus string

const (
	StrategyPlanStatusDraft      StrategyPlanStatus = "draft"
	StrategyPlanStatusActive     StrategyPlanStatus = "active"
	StrategyPlanStatusSuperseded StrategyPlanStatus = "superseded"
	StrategyPlanStatusArchived   StrategyPlanStatus = "archived"
)

type StrategyPlan struct {
	ID            uint
	CreatedAt     time.Time
	UpdatedAt     time.Time
	GeneratedAt   time.Time
	Status        StrategyPlanStatus
	AnalystModel  string
	PromptVersion string
	Watchlist     []string
	Summary       string
	Confidence    float64
}

type StrategyRecommendation struct {
	ID           uint
	PlanID       uint
	Symbol       string
	Bias         string
	Confidence   float64
	EntryMin     float64
	EntryMax     float64
	TargetPrice  float64
	StopPrice    float64
	MaxNotional  float64
	Thesis       string
	Invalidation string
	Priority     int
}

type StrategyPlanWithRecommendations struct {
	Plan            StrategyPlan
	Recommendations []StrategyRecommendation
}

type strategyPlanRecord struct {
	ID            uint      `gorm:"primaryKey"`
	GeneratedAt   time.Time `gorm:"index;not null"`
	Status        string    `gorm:"size:32;index;not null"`
	AnalystModel  string    `gorm:"size:128"`
	PromptVersion string    `gorm:"size:128"`
	WatchlistJSON string    `gorm:"type:text"`
	Summary       string    `gorm:"type:text"`
	Confidence    float64   `gorm:"not null;default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (strategyPlanRecord) TableName() string {
	return "strategy_plans"
}

type strategyRecommendationRecord struct {
	ID           uint    `gorm:"primaryKey"`
	PlanID       uint    `gorm:"index;not null"`
	Symbol       string  `gorm:"size:32;index;not null"`
	Bias         string  `gorm:"size:16;not null"`
	Confidence   float64 `gorm:"not null;default:0"`
	EntryMin     float64 `gorm:"not null;default:0"`
	EntryMax     float64 `gorm:"not null;default:0"`
	TargetPrice  float64 `gorm:"not null;default:0"`
	StopPrice    float64 `gorm:"not null;default:0"`
	MaxNotional  float64 `gorm:"not null;default:0"`
	Thesis       string  `gorm:"type:text"`
	Invalidation string  `gorm:"type:text"`
	Priority     int     `gorm:"not null;default:0;index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (strategyRecommendationRecord) TableName() string {
	return "strategy_recommendations"
}

type StrategyRepository struct {
	db *gorm.DB
}

func (r *StrategyRepository) CreatePlan(plan StrategyPlan, recommendations []StrategyRecommendation) (StrategyPlanWithRecommendations, error) {
	if r == nil || r.db == nil {
		return StrategyPlanWithRecommendations{}, fmt.Errorf("strategy repository is not initialized")
	}

	record, err := toStrategyPlanRecord(plan)
	if err != nil {
		return StrategyPlanWithRecommendations{}, err
	}
	if record.GeneratedAt.IsZero() {
		record.GeneratedAt = time.Now().UTC()
	}
	if strings.TrimSpace(record.Status) == "" {
		record.Status = string(StrategyPlanStatusDraft)
	}

	out := StrategyPlanWithRecommendations{}
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("insert strategy plan: %w", err)
		}
		recRecords, err := toStrategyRecommendationRecords(record.ID, recommendations)
		if err != nil {
			return err
		}
		if len(recRecords) > 0 {
			if err := tx.Create(&recRecords).Error; err != nil {
				return fmt.Errorf("insert strategy recommendations: %w", err)
			}
		}
		createdPlan, err := fromStrategyPlanRecord(record)
		if err != nil {
			return err
		}
		out.Plan = createdPlan
		out.Recommendations = fromStrategyRecommendationRecords(recRecords)
		return nil
	}); err != nil {
		return StrategyPlanWithRecommendations{}, err
	}
	return out, nil
}

func (r *StrategyRepository) SetPlanStatus(planID uint, status StrategyPlanStatus) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("strategy repository is not initialized")
	}
	statusValue := strings.ToLower(strings.TrimSpace(string(status)))
	if statusValue == "" {
		return fmt.Errorf("strategy plan status is required")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if statusValue == string(StrategyPlanStatusActive) {
			if err := tx.Model(&strategyPlanRecord{}).
				Where("status = ?", string(StrategyPlanStatusActive)).
				Updates(map[string]any{"status": string(StrategyPlanStatusSuperseded)}).Error; err != nil {
				return fmt.Errorf("demote active strategy plans: %w", err)
			}
		}
		result := tx.Model(&strategyPlanRecord{}).
			Where("id = ?", planID).
			Update("status", statusValue)
		if result.Error != nil {
			return fmt.Errorf("update strategy plan status: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("strategy plan %d not found", planID)
		}
		return nil
	})
}

func (r *StrategyRepository) GetActivePlan() (*StrategyPlanWithRecommendations, error) {
	return r.getSinglePlanByStatus(string(StrategyPlanStatusActive))
}

func (r *StrategyRepository) GetLatestPlan() (*StrategyPlanWithRecommendations, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	var record strategyPlanRecord
	result := r.db.
		Order("generated_at desc, id desc").
		Limit(1).
		Find(&record)
	if result.Error != nil {
		return nil, fmt.Errorf("query latest strategy plan: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	plan, err := fromStrategyPlanRecord(record)
	if err != nil {
		return nil, err
	}
	recs, err := r.ListRecommendations(record.ID)
	if err != nil {
		return nil, err
	}
	return &StrategyPlanWithRecommendations{
		Plan:            plan,
		Recommendations: recs,
	}, nil
}

func (r *StrategyRepository) ListRecentPlans(limit int) ([]StrategyPlan, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	if limit <= 0 {
		limit = defaultRecentStrategyPlanLimit
	}
	var records []strategyPlanRecord
	if err := r.db.
		Order("generated_at desc, id desc").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query strategy plans: %w", err)
	}
	out := make([]StrategyPlan, 0, len(records))
	for _, record := range records {
		plan, err := fromStrategyPlanRecord(record)
		if err != nil {
			return nil, err
		}
		out = append(out, plan)
	}
	return out, nil
}

func (r *StrategyRepository) ListRecommendations(planID uint) ([]StrategyRecommendation, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	var records []strategyRecommendationRecord
	if err := r.db.
		Where("plan_id = ?", planID).
		Order("priority asc, id asc").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query strategy recommendations: %w", err)
	}
	return fromStrategyRecommendationRecords(records), nil
}

func (r *StrategyRepository) getSinglePlanByStatus(status string) (*StrategyPlanWithRecommendations, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	var record strategyPlanRecord
	result := r.db.
		Where("status = ?", status).
		Order("generated_at desc, id desc").
		Limit(1).
		Find(&record)
	if result.Error != nil {
		return nil, fmt.Errorf("query strategy plan by status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	plan, err := fromStrategyPlanRecord(record)
	if err != nil {
		return nil, err
	}
	recs, err := r.ListRecommendations(record.ID)
	if err != nil {
		return nil, err
	}
	return &StrategyPlanWithRecommendations{
		Plan:            plan,
		Recommendations: recs,
	}, nil
}

func toStrategyPlanRecord(plan StrategyPlan) (strategyPlanRecord, error) {
	watchlistJSON := "[]"
	if len(plan.Watchlist) > 0 {
		b, err := json.Marshal(plan.Watchlist)
		if err != nil {
			return strategyPlanRecord{}, fmt.Errorf("marshal strategy watchlist: %w", err)
		}
		watchlistJSON = string(b)
	}
	return strategyPlanRecord{
		ID:            plan.ID,
		GeneratedAt:   plan.GeneratedAt.UTC(),
		Status:        strings.ToLower(strings.TrimSpace(string(plan.Status))),
		AnalystModel:  strings.TrimSpace(plan.AnalystModel),
		PromptVersion: strings.TrimSpace(plan.PromptVersion),
		WatchlistJSON: watchlistJSON,
		Summary:       strings.TrimSpace(plan.Summary),
		Confidence:    plan.Confidence,
	}, nil
}

func fromStrategyPlanRecord(record strategyPlanRecord) (StrategyPlan, error) {
	watchlist := make([]string, 0)
	raw := strings.TrimSpace(record.WatchlistJSON)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &watchlist); err != nil {
			return StrategyPlan{}, fmt.Errorf("unmarshal strategy watchlist: %w", err)
		}
	}
	return StrategyPlan{
		ID:            record.ID,
		CreatedAt:     record.CreatedAt.UTC(),
		UpdatedAt:     record.UpdatedAt.UTC(),
		GeneratedAt:   record.GeneratedAt.UTC(),
		Status:        StrategyPlanStatus(strings.ToLower(strings.TrimSpace(record.Status))),
		AnalystModel:  strings.TrimSpace(record.AnalystModel),
		PromptVersion: strings.TrimSpace(record.PromptVersion),
		Watchlist:     watchlist,
		Summary:       strings.TrimSpace(record.Summary),
		Confidence:    record.Confidence,
	}, nil
}

func toStrategyRecommendationRecords(planID uint, recommendations []StrategyRecommendation) ([]strategyRecommendationRecord, error) {
	records := make([]strategyRecommendationRecord, 0, len(recommendations))
	for _, rec := range recommendations {
		symbol := strings.ToUpper(strings.TrimSpace(rec.Symbol))
		if symbol == "" {
			return nil, fmt.Errorf("strategy recommendation symbol is required")
		}
		bias := strings.ToLower(strings.TrimSpace(rec.Bias))
		if bias == "" {
			bias = "hold"
		}
		records = append(records, strategyRecommendationRecord{
			PlanID:       planID,
			Symbol:       symbol,
			Bias:         bias,
			Confidence:   rec.Confidence,
			EntryMin:     rec.EntryMin,
			EntryMax:     rec.EntryMax,
			TargetPrice:  rec.TargetPrice,
			StopPrice:    rec.StopPrice,
			MaxNotional:  rec.MaxNotional,
			Thesis:       strings.TrimSpace(rec.Thesis),
			Invalidation: strings.TrimSpace(rec.Invalidation),
			Priority:     rec.Priority,
		})
	}
	return records, nil
}

func fromStrategyRecommendationRecords(records []strategyRecommendationRecord) []StrategyRecommendation {
	out := make([]StrategyRecommendation, 0, len(records))
	for _, rec := range records {
		out = append(out, StrategyRecommendation{
			ID:           rec.ID,
			PlanID:       rec.PlanID,
			Symbol:       strings.ToUpper(strings.TrimSpace(rec.Symbol)),
			Bias:         strings.ToLower(strings.TrimSpace(rec.Bias)),
			Confidence:   rec.Confidence,
			EntryMin:     rec.EntryMin,
			EntryMax:     rec.EntryMax,
			TargetPrice:  rec.TargetPrice,
			StopPrice:    rec.StopPrice,
			MaxNotional:  rec.MaxNotional,
			Thesis:       strings.TrimSpace(rec.Thesis),
			Invalidation: strings.TrimSpace(rec.Invalidation),
			Priority:     rec.Priority,
		})
	}
	return out
}
