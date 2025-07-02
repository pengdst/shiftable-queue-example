package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrate(t *testing.T) {
	t.Run("POSITIVE-MigrateSuccess", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		assert.NoError(t, err, "failed to open in-memory db")
		Migrate(db)
		// Check if table created
		assert.True(t, db.Migrator().HasTable(&Queue{}))
	})
}
