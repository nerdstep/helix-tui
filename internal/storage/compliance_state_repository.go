package storage

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type complianceUnsettledSellRecord struct {
	ID        uint      `gorm:"primaryKey"`
	Amount    float64   `gorm:"not null"`
	SettlesAt time.Time `gorm:"index;not null"`
	CreatedAt time.Time `gorm:"index;not null"`
}

func (complianceUnsettledSellRecord) TableName() string {
	return "compliance_unsettled_sells"
}

type ComplianceUnsettledSell struct {
	Amount    float64
	SettlesAt time.Time
	CreatedAt time.Time
}

type ComplianceStateRepository struct {
	db *gorm.DB
}

func (r *ComplianceStateRepository) AppendUnsettledSell(lot ComplianceUnsettledSell) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("compliance state repository is not initialized")
	}
	if lot.Amount <= 0 {
		return fmt.Errorf("unsettled sell amount must be > 0")
	}
	if lot.SettlesAt.IsZero() {
		return fmt.Errorf("unsettled sell settles_at is required")
	}
	if lot.CreatedAt.IsZero() {
		lot.CreatedAt = time.Now().UTC()
	}
	record := complianceUnsettledSellRecord{
		Amount:    lot.Amount,
		SettlesAt: lot.SettlesAt.UTC(),
		CreatedAt: lot.CreatedAt.UTC(),
	}
	if err := r.db.Create(&record).Error; err != nil {
		return fmt.Errorf("insert compliance unsettled sell: %w", err)
	}
	return nil
}

func (r *ComplianceStateRepository) ListUnsettledSells(asOf time.Time) ([]ComplianceUnsettledSell, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("compliance state repository is not initialized")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	asOf = asOf.UTC()

	var records []complianceUnsettledSellRecord
	if err := r.db.
		Where("settles_at > ?", asOf).
		Order("settles_at asc, id asc").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query compliance unsettled sells: %w", err)
	}

	out := make([]ComplianceUnsettledSell, 0, len(records))
	for _, rec := range records {
		out = append(out, ComplianceUnsettledSell{
			Amount:    rec.Amount,
			SettlesAt: rec.SettlesAt.UTC(),
			CreatedAt: rec.CreatedAt.UTC(),
		})
	}
	return out, nil
}

func (r *ComplianceStateRepository) PruneSettledSells(asOf time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("compliance state repository is not initialized")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	asOf = asOf.UTC()
	if err := r.db.
		Where("settles_at <= ?", asOf).
		Delete(&complianceUnsettledSellRecord{}).Error; err != nil {
		return fmt.Errorf("delete settled compliance sells: %w", err)
	}
	return nil
}
