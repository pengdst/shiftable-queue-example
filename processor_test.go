package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// Helper untuk setup processor test
func setupProcessorTest(t *testing.T) (*QueueProcessor, *MockPublisher, *MockCloser, *MockRestAPI, *gorm.DB, func()) {
	db, dbCleanup := setupTestDatabase(t)
	mockCloser := NewMockCloser(t)
	mockPublisher := NewMockPublisher(t)
	mockAPI := NewMockRestAPI(t)

	processor, err := NewQueueProcessor(
		nil, // config is not needed in this test
		db,
		mockAPI,
		WithTestConnection(mockCloser, mockPublisher),
	)
	assert.NoError(t, err)

	// Mock Close() for cleanup to prevent unexpected calls
	mockPublisher.EXPECT().Close().Return(nil).Maybe()
	mockCloser.EXPECT().Close().Return(nil).Maybe()

	cleanup := func() {
		processor.Stop()
		dbCleanup()
	}
	return processor, mockPublisher, mockCloser, mockAPI, db, cleanup
}

func TestQueueProcessor_ProcessEligibleQueue(t *testing.T) {
	t.Run("POSITIVE-Success", func(t *testing.T) {
		processor, _, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		queue := &Queue{Name: "q-success", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)

		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()

		err := processor.ProcessEligibleQueue(context.Background())
		assert.NoError(t, err)

		var updated Queue
		_ = db.First(&updated, queue.ID).Error
		assert.Equal(t, StatusCompleted, updated.Status)
		assert.Equal(t, 0, updated.RetryCount)
		mockAPI.AssertExpectations(t)
	})

	t.Run("NEGATIVE-Failure", func(t *testing.T) {
		processor, _, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		queue := &Queue{Name: "q-fail", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)

		errFail := assert.AnError
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Return(errFail).Once()

		err := processor.ProcessEligibleQueue(context.Background())
		assert.Error(t, err)

		var updated Queue
		_ = db.First(&updated, queue.ID).Error
		assert.Equal(t, StatusFailed, updated.Status)
		assert.Equal(t, 1, updated.RetryCount)
		assert.True(t, updated.LastRetryAt.Valid)
		mockAPI.AssertExpectations(t)
	})

	t.Run("NEGATIVE-EmptyQueue", func(t *testing.T) {
		processor, _, _, mockAPI, _, cleanup := setupProcessorTest(t)
		defer cleanup()

		err := processor.ProcessEligibleQueue(context.Background())
		assert.NoError(t, err)
		// Nothing to assert in DB, just make sure no panic/error
		mockAPI.AssertExpectations(t)
	})

	t.Run("POSITIVE-ChainProcessing", func(t *testing.T) {
		processor, mockPublisher, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		for i := range 3 {
			_ = repo.Save(context.Background(), &Queue{Name: fmt.Sprintf("q-%d", i), Status: StatusPending})
		}
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Times(3)
		mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

		for range 3 {
			err := processor.ProcessEligibleQueue(context.Background())
			assert.NoError(t, err)
		}

		var queues []Queue
		db.Find(&queues)
		for _, q := range queues {
			assert.Equal(t, StatusCompleted, q.Status)
		}
		mockAPI.AssertExpectations(t)
	})

	t.Run("NEGATIVE-GetEligibleQueues_DBError", func(t *testing.T) {
		processor, _, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		// Close DB to simulate error
		sqlDB, _ := db.DB()
		sqlDB.Close()
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Maybe()
		err := processor.ProcessEligibleQueue(context.Background())
		assert.Error(t, err)
	})

	t.Run("NEGATIVE-SaveQueue_DBError", func(t *testing.T) {
		processor, _, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		queue := &Queue{Name: "q-db-error", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Run(func(queue *Queue) {
			sqlDB, _ := db.DB()
			sqlDB.Close()
		}).Return(nil).Once()
		err := processor.ProcessEligibleQueue(context.Background())
		assert.Error(t, err)
	})

	t.Run("NEGATIVE-SimulateProcessingErr_SaveQueueDBError", func(t *testing.T) {
		processor, _, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		queue := &Queue{Name: "q-db-error", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Run(func(queue *Queue) {
			sqlDB, _ := db.DB()
			sqlDB.Close()
		}).Return(assert.AnError).Once()
		err := processor.ProcessEligibleQueue(context.Background())
		assert.Error(t, err)
	})

	t.Run("NEGATIVE-ChainProcessing_DBError", func(t *testing.T) {
		processor, mockPublisher, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		// Insert 2 queue
		_ = repo.Save(context.Background(), &Queue{Name: "q1", Status: StatusPending})
		_ = repo.Save(context.Background(), &Queue{Name: "q2", Status: StatusPending})
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()
		// Close DB after first process, before chain
		err := processor.ProcessEligibleQueue(context.Background())
		assert.NoError(t, err)
		sqlDB, _ := db.DB()
		sqlDB.Close()
		err = processor.ProcessEligibleQueue(context.Background())
		assert.Error(t, err)
	})

	t.Run("NEGATIVE-PublishWithContextError", func(t *testing.T) {
		processor, mockPublisher, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		queue := &Queue{Name: "q-pub-error", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Twice()
		mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(assert.AnError).Once()
		// Insert 2 queue biar chain processing jalan
		_ = repo.Save(context.Background(), &Queue{Name: "q-pub-error2", Status: StatusPending})
		err := processor.ProcessEligibleQueue(context.Background())
		assert.NoError(t, err) // proses pertama sukses
		err = processor.ProcessEligibleQueue(context.Background())
		assert.NoError(t, err) // proses kedua tetap jalan, error publish hanya log
	})
}

func TestQueueProcessor_TriggerProcessing(t *testing.T) {
	t.Run("POSITIVE-PublishSuccess", func(t *testing.T) {
		processor, mockPublisher, _, _, _, cleanup := setupProcessorTest(t)
		defer cleanup()
		mockPublisher.EXPECT().
			PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).
			Return(nil).Once()
		err := processor.TriggerProcessing(context.Background())
		assert.NoError(t, err)
		mockPublisher.AssertExpectations(t)
	})

	t.Run("NEGATIVE-PublishError", func(t *testing.T) {
		processor, mockPublisher, _, _, _, cleanup := setupProcessorTest(t)
		defer cleanup()
		errExpected := assert.AnError
		mockPublisher.EXPECT().
			PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).
			Return(errExpected).Once()
		err := processor.TriggerProcessing(context.Background())
		assert.Error(t, err)
		assert.Equal(t, errExpected, err)
		mockPublisher.AssertExpectations(t)
	})
}

func TestQueueProcessor_Stop(t *testing.T) {
	// Re-setup mocks without using the shared setupProcessorTest to avoid
	// conflicting/duplicate mock expectations on the .Close() method.
	db, dbCleanup := setupTestDatabase(t)
	defer dbCleanup()

	mockCloser := NewMockCloser(t)
	mockPublisher := NewMockPublisher(t)
	mockAPI := NewMockRestAPI(t)
	processor, err := NewQueueProcessor(
		nil, // config is not needed in this test
		db,
		mockAPI,
		WithTestConnection(mockCloser, mockPublisher),
	)
	assert.NoError(t, err)

	// Set clear expectations for this specific test.
	mockPublisher.EXPECT().Close().Return(nil).Once()
	mockCloser.EXPECT().Close().Return(nil).Once()

	// Execute the function under test.
	processor.Stop()

	// Assert that the expectations were met.
	mockPublisher.AssertExpectations(t)
	mockCloser.AssertExpectations(t)
}

// mockAcknowledger implements the amqp091.Acknowledger interface for testing.
type mockAcknowledger struct {
	acked  chan bool
	nacked chan bool
}

func (a *mockAcknowledger) Ack(tag uint64, multiple bool) error {
	// In this test, we don't expect Ack to be called, but it's good practice to implement it.
	// For simplicity, we won't signal anything here.
	return nil
}

func (a *mockAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	// Signal that Nack was called.
	a.nacked <- true
	return nil
}

func (a *mockAcknowledger) Reject(tag uint64, requeue bool) error {
	// In this test, we don't expect Reject to be called.
	return nil
}

func TestQueueProcessor_Start(t *testing.T) {
	t.Run("POSITIVE-ProcessMessage", func(t *testing.T) {
		processor, mockPublisher, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()

		// Setup: insert eligible queue
		repo := NewRepository(db)
		queue := &Queue{Name: "start-queue", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)

		// Setup mock Consume
		msgCh := make(chan amqp091.Delivery, 1)
		mockPublisher.EXPECT().Consume(
			"queue_processing", "", false, false, false, false, mock.Anything,
		).Return((<-chan amqp091.Delivery)(msgCh), nil).Once()

		// Setup mock SimulateProcessing
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

		// Run Start in a goroutine and wait for it to complete
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			processor.Start()
		}()

		// Insert a dummy message
		msgCh <- amqp091.Delivery{}
		close(msgCh)

		wg.Wait() // Wait for the processor to finish

		// Assert effects: status queue should be Completed
		var updated Queue
		_ = db.First(&updated, queue.ID).Error
		assert.Equal(t, StatusCompleted, updated.Status)
		mockPublisher.AssertExpectations(t)
		mockAPI.AssertExpectations(t)
	})

	t.Run("NEGATIVE-ProcessEligibleQueueError_NackMessage", func(t *testing.T) {
		processor, mockPublisher, _, mockAPI, db, _ := setupProcessorTest(t)
		// No defer cleanup() here because we need to control the Stop() call manually.

		repo := NewRepository(db)
		queue := &Queue{Name: "start-nack", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)

		// Setup the mock acknowledger to spy on Nack calls.
		acknowledger := &mockAcknowledger{
			nacked: make(chan bool, 1),
		}

		// Create a delivery message with our mock acknowledger.
		delivery := amqp091.Delivery{
			Acknowledger: acknowledger,
			Body:         []byte("test message"),
		}

		msgCh := make(chan amqp091.Delivery, 1)
		msgCh <- delivery

		mockPublisher.EXPECT().Consume(
			"queue_processing", "", false, false, false, false, mock.Anything,
		).Return((<-chan amqp091.Delivery)(msgCh), nil).Once()
		mockAPI.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Once()

		// Run Start in a goroutine.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			processor.Start()
		}()

		select {
		case <-acknowledger.nacked:
			// Success: Nack was called as expected.
		case <-time.After(2 * time.Second):
			assert.Fail(t, "timed out waiting for Nack to be called")
		}

		// Manually stop the processor and wait for the goroutine to exit.
		close(msgCh)
		processor.Stop()
		wg.Wait()

		// Final assertions.
		mockPublisher.AssertExpectations(t)
		mockAPI.AssertExpectations(t)
	})
}

func TestQueueProcessor_INTEGRATION_ShiftingQueue_AntiStarvation(t *testing.T) {
	processor, mockPublisher, _, apiMock, db, cleanup := setupProcessorTest(t)
	defer cleanup()
	repo := NewRepository(db)
	mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

	baseTime := time.Now().Add(-1 * time.Hour)
	queue1 := &Queue{Name: "queue-oldest", Status: StatusPending}
	_ = repo.Save(context.Background(), queue1)
	db.Model(queue1).Update("created_at", baseTime)
	queue2 := &Queue{Name: "queue-middle", Status: StatusPending}
	_ = repo.Save(context.Background(), queue2)
	db.Model(queue2).Update("created_at", baseTime.Add(30*time.Minute))
	queue3 := &Queue{Name: "queue-newest", Status: StatusPending}
	_ = repo.Save(context.Background(), queue3)
	db.Model(queue3).Update("created_at", baseTime.Add(60*time.Minute))

	// Step 1: Process (should get oldest, fail)
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Once()
	_ = processor.ProcessEligibleQueue(context.Background())

	// Step 2: Process (should get middle, success)
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
	_ = processor.ProcessEligibleQueue(context.Background())

	// Step 3: Process (should get newest, success)
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
	_ = processor.ProcessEligibleQueue(context.Background())

	// Step 4: Process (should get failed oldest again, success)
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
	_ = processor.ProcessEligibleQueue(context.Background())

	// Cek status akhir queue di DB
	var q1, q2, q3 Queue
	_ = db.Where("name = ?", "queue-oldest").First(&q1).Error
	_ = db.Where("name = ?", "queue-middle").First(&q2).Error
	_ = db.Where("name = ?", "queue-newest").First(&q3).Error

	assert.Equal(t, StatusCompleted, q1.Status)
	assert.Equal(t, StatusCompleted, q2.Status)
	assert.Equal(t, StatusCompleted, q3.Status)
}

func TestQueueProcessor_INTEGRATION_Starvation_BurstInsert(t *testing.T) {
	processor, mockPublisher, _, apiMock, db, cleanup := setupProcessorTest(t)
	defer cleanup()
	repo := NewRepository(db)
	mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

	// Step 1: Insert satu queue gagal
	queueFail := &Queue{Name: "queue-fail", Status: StatusPending}
	_ = repo.Save(context.Background(), queueFail)
	// Step 2: Insert 10 queue baru
	for i := 0; i < 10; i++ {
		q := &Queue{Name: fmt.Sprintf("queue-burst-%d", i), Status: StatusPending}
		_ = repo.Save(context.Background(), q)
	}
	// Step 3: Fail queue-fail di proses pertama
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Once()
	_ = processor.ProcessEligibleQueue(context.Background())
	// Step 4: Semua queue lain success
	for i := 0; i < 10; i++ {
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
	}
	// Step 5: Retry queue-fail, success
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
	_ = processor.ProcessEligibleQueue(context.Background())
	// Step 6: Assert queue-fail completed
	var qf Queue
	_ = db.Where("name = ?", "queue-fail").First(&qf).Error
	assert.Equal(t, StatusCompleted, qf.Status)
	assert.Equal(t, 1, qf.RetryCount)
}

func TestQueueProcessor_INTEGRATION_MultipleFailures_RetryCount(t *testing.T) {
	processor, mockPublisher, _, apiMock, db, cleanup := setupProcessorTest(t)
	defer cleanup()
	repo := NewRepository(db)
	mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

	queue := &Queue{Name: "queue-retry", Status: StatusPending}
	_ = repo.Save(context.Background(), queue)
	// Fail 2x
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Twice()
	_ = processor.ProcessEligibleQueue(context.Background())
	_ = processor.ProcessEligibleQueue(context.Background())
	// Success
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
	_ = processor.ProcessEligibleQueue(context.Background())
	// Assert
	var qr Queue
	_ = db.Where("name = ?", "queue-retry").First(&qr).Error
	assert.Equal(t, StatusCompleted, qr.Status)
	assert.Equal(t, 2, qr.RetryCount)
}

func TestQueueProcessor_INTEGRATION_InterleavedSuccessFailure(t *testing.T) {
	processor, mockPublisher, _, apiMock, db, cleanup := setupProcessorTest(t)
	defer cleanup()
	repo := NewRepository(db)
	mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

	// Step 1: Insert Queue1 & Queue2
	queue1 := &Queue{Name: "queue1", Status: StatusPending}
	queue2 := &Queue{Name: "queue2", Status: StatusPending}
	_ = repo.Save(context.Background(), queue1)
	_ = repo.Save(context.Background(), queue2)
	// Step 2: Queue1 gagal
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Once()
	_ = processor.ProcessEligibleQueue(context.Background())
	// Step 3: Queue2 sukses
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
	_ = processor.ProcessEligibleQueue(context.Background())
	// Step 4: Insert Queue3 setelah proses mulai
	queue3 := &Queue{Name: "queue3", Status: StatusPending}
	_ = repo.Save(context.Background(), queue3)
	// Step 5: Queue1 retry sukses
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
	_ = processor.ProcessEligibleQueue(context.Background())
	// Step 6: Queue3 sukses
	apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
	_ = processor.ProcessEligibleQueue(context.Background())
	// Assert status akhir
	var q1, q2, q3 Queue
	_ = db.Where("name = ?", "queue1").First(&q1).Error
	_ = db.Where("name = ?", "queue2").First(&q2).Error
	_ = db.Where("name = ?", "queue3").First(&q3).Error
	assert.Equal(t, StatusCompleted, q1.Status)
	assert.Equal(t, 1, q1.RetryCount)
	assert.Equal(t, StatusCompleted, q2.Status)
	assert.Equal(t, 0, q2.RetryCount)
	assert.Equal(t, StatusCompleted, q3.Status)
	assert.Equal(t, 0, q3.RetryCount)
}

func TestQueueProcessor_INTEGRATION_AllQueuesFailedThenSucceed(t *testing.T) {
	processor, mockPublisher, _, apiMock, db, cleanup := setupProcessorTest(t)
	defer cleanup()
	repo := NewRepository(db)
	mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

	// Step 1: Insert 3 queue
	queues := []*Queue{
		{Name: "fail1", Status: StatusPending},
		{Name: "fail2", Status: StatusPending},
		{Name: "fail3", Status: StatusPending},
	}
	for _, q := range queues {
		_ = repo.Save(context.Background(), q)
	}
	// Step 2: Semua gagal
	for range 3 {
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
	}
	// Step 3: Semua retry sukses
	for range 3 {
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
	}
	// Assert status akhir dan retry count
	for _, name := range []string{"fail1", "fail2", "fail3"} {
		var q Queue
		_ = db.Where("name = ?", name).First(&q).Error
		assert.Equal(t, StatusCompleted, q.Status)
		assert.Equal(t, 1, q.RetryCount)
	}
}

// --- END: Shifting/Anti-Starvation tests ---
