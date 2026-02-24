package storage

import (
	"fmt"

	"gorm.io/gorm"
)

func runMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&equityHistoryRecord{},
		&tradeEventRecord{},
		&complianceUnsettledSellRecord{},
		&strategyPlanRecord{},
		&strategyRecommendationRecord{},
		&strategyChatThreadRecord{},
		&strategyChatMessageRecord{},
		&strategySteeringStateRecord{},
		&strategySteeringSymbolRecord{},
	); err != nil {
		return fmt.Errorf("auto-migrate database schema: %w", err)
	}
	return nil
}
