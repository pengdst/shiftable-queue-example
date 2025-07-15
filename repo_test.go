package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRepo_Save(t *testing.T) {
	t.Run("POSITIVE-SaveNewQueue_ReturnsNoError", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		queue := &Queue{Name: "test-save"}
		err := repo.Save(context.Background(), queue)
		assert.NoError(t, err)
		assert.NotZero(t, queue.ID)

		var foundQueue Queue
		db.First(&foundQueue, queue.ID)
		assert.Equal(t, "test-save", foundQueue.Name)
	})

	t.Run("POSITIVE-UpdateExistingQueue_ReturnsNoError", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		queue := &Queue{Name: "test-update", Status: StatusPending}
		db.Create(queue)

		queue.Status = StatusCompleted
		err := repo.Save(context.Background(), queue)
		assert.NoError(t, err)

		var foundQueue Queue
		db.First(&foundQueue, queue.ID)
		assert.Equal(t, StatusCompleted, foundQueue.Status)
	})
}

func TestRepo_Fetch(t *testing.T) {
	t.Run("POSITIVE-FetchWithData_ReturnsSortedQueues", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		q1 := &Queue{Name: "q1", Status: StatusPending}
		q2 := &Queue{Name: "q2", Status: StatusCompleted}
		db.Create(q2) // Create q2 first
		// Manually update q2's timestamp to be older
		db.Model(&q2).Update("created_at", time.Now().Add(-1*time.Hour))
		db.Create(q1) // Then create q1, making it newer

		queues, err := repo.Fetch(context.Background())
		assert.NoError(t, err)
		assert.Len(t, queues, 2)
		// Should be sorted by status first (pending), then created_at
		assert.Equal(t, "q1", queues[0].Name) // Pending comes before Completed
		assert.Equal(t, "q2", queues[1].Name)
	})

	t.Run("POSITIVE-FetchEmpty_ReturnsEmptySlice", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		queues, err := repo.Fetch(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, queues)
	})
}

func TestRepo_Delete(t *testing.T) {
	t.Run("ByID-POSITIVE-Found_DeletesAndReturnsNoError", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()
		repo := NewRepository(db)
		queue := &Queue{Name: "test-delete-id"}
		db.Create(queue)

		err := repo.DeleteByID(context.Background(), int(queue.ID))
		assert.NoError(t, err)

		var foundQueue Queue
		err = db.First(&foundQueue, queue.ID).Error
		assert.Error(t, err) // Should be gorm.ErrRecordNotFound
	})

	t.Run("ByID-NEGATIVE-NotFound_ReturnsError", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()
		repo := NewRepository(db)
		err := repo.DeleteByID(context.Background(), 999)
		assert.Error(t, err)
	})

	t.Run("ByName-POSITIVE-Found_DeletesAndReturnsNoError", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()
		repo := NewRepository(db)
		queue := &Queue{Name: "test-delete-name"}
		db.Create(queue)

		err := repo.DeleteByName(context.Background(), "test-delete-name")
		assert.NoError(t, err)

		var foundQueue Queue
		err = db.Where("name = ?", "test-delete-name").First(&foundQueue).Error
		assert.Error(t, err)
	})

	t.Run("ByName-NEGATIVE-NotFound_ReturnsError", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()
		repo := NewRepository(db)
		err := repo.DeleteByName(context.Background(), "not-found-name")
		assert.Error(t, err)
	})
}

func TestRepo_Get(t *testing.T) {
	t.Run("ByName-POSITIVE-Found_ReturnsQueue", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()
		repo := NewRepository(db)
		queue := &Queue{Name: "test-get-name"}
		db.Create(queue)

		foundQueue, err := repo.GetQueueByName(context.Background(), "test-get-name")
		assert.NoError(t, err)
		assert.NotNil(t, foundQueue)
		assert.Equal(t, "test-get-name", foundQueue.Name)
	})

	t.Run("ByName-NEGATIVE-NotFound_ReturnsError", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()
		repo := NewRepository(db)
		_, err := repo.GetQueueByName(context.Background(), "not-found-name")
		assert.Error(t, err)
	})

	t.Run("Eligible-POSITIVE-Found_ReturnsEligibleQueue", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()
		repo := NewRepository(db)

		q1 := &Queue{Name: "q-completed", Status: StatusCompleted}
		q2 := &Queue{Name: "q-pending-old", Status: StatusPending}
		q3 := &Queue{Name: "q-pending-new", Status: StatusPending}
		db.Create(q1)
		db.Create(q2)
		// Manually update q2's timestamp to be older
		db.Model(&q2).Update("created_at", time.Now().Add(-1*time.Hour))
		db.Create(q3)

		eligible, err := repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Len(t, eligible, 1)
		assert.Equal(t, "q-pending-old", eligible[0].Name) // Should get the oldest pending one
	})

	t.Run("Eligible-POSITIVE-Empty_ReturnsEmptySlice", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()
		repo := NewRepository(db)
		q1 := &Queue{Name: "q-completed", Status: StatusCompleted}
		db.Create(q1)

		eligible, err := repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, eligible)
	})
}

func TestRepo_HasRemainingQueues(t *testing.T) {
	t.Run("POSITIVE-HasRemainingQueue_ReturnTrue", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		err := db.Create(&Queue{Status: StatusPending}).Error
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

	t.Run("NEGATIVE-NoEligibleQueue_ReturnFalse", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		err := db.Create(&Queue{Status: StatusCompleted}).Error
		assert.NoError(t, err)
		exists, err := repo.HasRemainingQueues(context.Background())
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("NEGATIVE-DBError_ReturnsError", func(t *testing.T) {
		db, shutdown := setupTestDatabase(t)
		defer shutdown()

		repo := NewRepository(db)
		sqlDB, _ := db.DB()
		sqlDB.Close()

		exists, err := repo.HasRemainingQueues(context.Background())
		assert.Error(t, err)
		assert.False(t, exists)
	})
}
