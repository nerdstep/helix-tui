package storage

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type schemaMigration struct {
	Version   string    `gorm:"primaryKey;size:128"`
	AppliedAt time.Time `gorm:"not null"`
}

func (schemaMigration) TableName() string {
	return "schema_migrations"
}

type migration struct {
	version string
	apply   func(*gorm.DB) error
}

var migrations = []migration{
	{
		version: "20260219_create_equity_history",
		apply: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&equityHistoryRecord{})
		},
	},
}

func runMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return fmt.Errorf("migrate schema_migrations: %w", err)
	}

	var appliedRows []schemaMigration
	if err := db.Find(&appliedRows).Error; err != nil {
		return fmt.Errorf("list applied migrations: %w", err)
	}
	applied := make(map[string]struct{}, len(appliedRows))
	for _, row := range appliedRows {
		applied[row.Version] = struct{}{}
	}

	for _, m := range migrations {
		if _, ok := applied[m.version]; ok {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := m.apply(tx); err != nil {
				return err
			}
			return tx.Create(&schemaMigration{
				Version:   m.version,
				AppliedAt: time.Now().UTC(),
			}).Error
		}); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.version, err)
		}
	}
	return nil
}
