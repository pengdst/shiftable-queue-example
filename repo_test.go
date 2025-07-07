package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepo_HasRemainingQueues(t *testing.T) {
	t.Run("POSITIVE-HasRemainingQueue_ReturnTrue", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		err := db.Create(&Queue{}).Error
		assert.NoError(t, err)
		exists, err := repo.HasRemainingQueues(context.Background())
		assert.NoError(t, err)
		assert.True(t, exists)
	})
	t.Run("NEGATIVE-EmptyQueue_ReturnFalse", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		exists, err := repo.HasRemainingQueues(context.Background())
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}
