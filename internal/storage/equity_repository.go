package storage

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type EquityPoint struct {
	Time   time.Time
	Equity float64
}

type equityHistoryRecord struct {
	ID        uint      `gorm:"primaryKey"`
	Time      time.Time `gorm:"index;not null"`
	Equity    float64   `gorm:"not null"`
	CreatedAt time.Time
}

func (equityHistoryRecord) TableName() string {
	return "equity_history"
}

type EquityRepository struct {
	db *gorm.DB
}

func (r *EquityRepository) Append(point EquityPoint) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("equity repository is not initialized")
	}
	record := equityHistoryRecord{
		Time:   point.Time.UTC(),
		Equity: point.Equity,
	}
	if err := r.db.Create(&record).Error; err != nil {
		return fmt.Errorf("insert equity history: %w", err)
	}
	return nil
}

func (r *EquityRepository) List() ([]EquityPoint, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("equity repository is not initialized")
	}
	var records []equityHistoryRecord
	if err := r.db.Order("time asc").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query equity history: %w", err)
	}
	out := make([]EquityPoint, 0, len(records))
	for _, rec := range records {
		out = append(out, EquityPoint{
			Time:   rec.Time.UTC(),
			Equity: rec.Equity,
		})
	}
	return out, nil
}
