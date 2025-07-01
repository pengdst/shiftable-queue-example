package main

import (
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	err := db.AutoMigrate(&Queue{})
	if err != nil {
		log.Fatal().Msgf("failed to run migration: %s", err.Error())
	}
}
