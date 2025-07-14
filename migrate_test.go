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
		err = Migrate(db)
		assert.NoError(t, err)
		// Check if table created
		assert.True(t, db.Migrator().HasTable(&Queue{}))
	})

	t.Run("POSITIVE-MigrateNoOp", func(t *testing.T) {
		// Open in-memory SQLite database
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		assert.NoError(t, err, "failed to open in-memory db")

		// First migration should succeed
		err = Migrate(db)
		assert.NoError(t, err, "first migration failed")
		assert.True(t, db.Migrator().HasTable(&Queue{}))

		// Second migration (no-op) should also succeed
		err = Migrate(db)
		assert.NoError(t, err, "second migration (no-op) failed")
		assert.True(t, db.Migrator().HasTable(&Queue{}))
	})

	t.Run("NEGATIVE-MigrateError_NilDB", func(t *testing.T) {
		// Now that Migrate returns an error, we can test this more gracefully
		err := Migrate(nil)
		assert.Error(t, err)
	})
}