package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// Helper untuk setup processor test
func setupProcessorTest(t *testing.T) (*QueueProcessor, *MockPublisher, *MockRestAPI, *gorm.DB, func()) {
	db, dbCleanup := setupTestDatabase(t)
	mq, _, _, mqCleanup := setupRabbitMQTest(t)
	mockPublisher := NewMockPublisher(t)
	mockAPI := NewMockRestAPI(t)
	processor := &QueueProcessor{
		repo:    NewRepository(db),
		conn:    mq,
		channel: mockPublisher,
		api:     mockAPI,
	}
	cleanup := func() {
		dbCleanup()
		mqCleanup()
	}
	return processor, mockPublisher, mockAPI, db, cleanup
}

func TestQueueProcessor_ProcessEligibleQueue(t *testing.T) {
	t.Run("POSITIVE-Success", func(t *testing.T) {
		processor, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		queue := &Queue{Name: "q-success", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)

		mockAPI.EXPECT().SimulateProcessing(mock.AnythingOfType("*main.Queue")).Return(nil).Once()

		err := processor.ProcessEligibleQueue(context.Background())
		assert.NoError(t, err)

		var updated Queue
		_ = db.First(&updated, queue.ID).Error
		assert.Equal(t, StatusCompleted, updated.Status)
		assert.Equal(t, 0, updated.RetryCount)
		mockAPI.AssertExpectations(t)
	})

	t.Run("NEGATIVE-Failure", func(t *testing.T) {
		processor, _, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		queue := &Queue{Name: "q-fail", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)

		errFail := assert.AnError
		mockAPI.EXPECT().SimulateProcessing(mock.AnythingOfType("*main.Queue")).Return(errFail).Once()

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
		processor, _, mockAPI, _, cleanup := setupProcessorTest(t)
		defer cleanup()

		err := processor.ProcessEligibleQueue(context.Background())
		assert.NoError(t, err)
		// Nothing to assert in DB, just make sure no panic/error
		mockAPI.AssertExpectations(t)
	})

	t.Run("POSITIVE-ChainProcessing", func(t *testing.T) {
		processor, mockPublisher, mockAPI, db, cleanup := setupProcessorTest(t)
		defer cleanup()
		repo := NewRepository(db)
		for i := 0; i < 3; i++ {
			_ = repo.Save(context.Background(), &Queue{Name: fmt.Sprintf("q-%d", i), Status: StatusPending})
		}
		mockAPI.EXPECT().SimulateProcessing(mock.AnythingOfType("*main.Queue")).Return(nil).Times(3)
		mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

		for i := 0; i < 3; i++ {
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
}

func TestQueueProcessor_TriggerProcessing(t *testing.T) {
	t.Run("POSITIVE-PublishSuccess", func(t *testing.T) {
		processor, mockPublisher, _, _, cleanup := setupProcessorTest(t)
		defer cleanup()
		mockPublisher.EXPECT().
			PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).
			Return(nil).Once()
		err := processor.TriggerProcessing(context.Background())
		assert.NoError(t, err)
		mockPublisher.AssertExpectations(t)
	})

	t.Run("NEGATIVE-PublishError", func(t *testing.T) {
		processor, mockPublisher, _, _, cleanup := setupProcessorTest(t)
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
	processor, mockPublisher, _, _, cleanup := setupProcessorTest(t)
	defer cleanup()

	mockPublisher.EXPECT().Close().Return(nil).Once()
	mockConn := NewMockCloser(t)
	mockConn.EXPECT().Close().Return(nil).Once()
	processor.conn = mockConn

	processor.Stop()
	mockPublisher.AssertExpectations(t)
	mockConn.AssertExpectations(t)
}

func TestQueueProcessor_Start(t *testing.T) {
	t.Run("POSITIVE-ProcessMessage", func(t *testing.T) {
		processor, mockPublisher, mockAPI, db, cleanup := setupProcessorTest(t)
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
		mockAPI.EXPECT().SimulateProcessing(mock.AnythingOfType("*main.Queue")).Return(nil).Once()
		mockPublisher.EXPECT().PublishWithContext(mock.Anything, "", "queue_processing", false, false, mock.Anything).Return(nil).Maybe()

		// Insert a dummy message
		msgCh <- amqp091.Delivery{}
		close(msgCh)

		// Run Start (should process message)
		go func() { processor.Start() }()
		// Wait sebentar biar goroutine jalan
		time.Sleep(100 * time.Millisecond)

		// Assert efek: status queue berubah jadi Completed
		var updated Queue
		_ = db.First(&updated, queue.ID).Error
		assert.Equal(t, StatusCompleted, updated.Status)
		mockPublisher.AssertExpectations(t)
		mockAPI.AssertExpectations(t)
	})
}
