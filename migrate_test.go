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

	t.Run("NEGATIVE-MigrateError_AutoMigrateFails", func(t *testing.T) {
		// Use a mock dialector that will cause AutoMigrate to fail.
		// This is a bit tricky as we need to simulate a failure at the GORM level.
		// One way is to use a real DB but with an invalid table structure or permissions.
		// For simplicity, we'll use a mock that simulates the error.
		// NOTE: This requires a more advanced mocking of the gorm.DB which is complex.
		// A simpler approach for this specific case is to use a closed DB connection.
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		assert.NoError(t, err)
		sqlDB, _ := db.DB()
		sqlDB.Close() // Close the underlying connection

		err = Migrate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to run migration")
	})
}
