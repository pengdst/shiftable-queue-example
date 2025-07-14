package main

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("gorm.DB instance is nil")
	}
	err := db.AutoMigrate(&Queue{})
	if err != nil {
		log.Error().Msgf("failed to run migration: %s", err.Error())
		return fmt.Errorf("failed to run migration: %w", err)
	}
	return nil
}
