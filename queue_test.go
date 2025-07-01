package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type queueResp struct {
	Data []Queue `json:"data"`
}

type errorResp struct {
	Error string `json:"error"`
}

// Minimal test factory - inject database instance to server
func setupTestServer() (*httptest.Server, func()) {
	// Set required RABBITMQ_URL for test environment
	os.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	cfg := Load()
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: NewLogLevel(cfg.LogLevel),
	})
	server := NewServer(cfg, db)

	ts := httptest.NewServer(server.router)

	cleanup := func() {
		// Truncate queues table biar ga interfere test lain
		db.Exec("TRUNCATE TABLE queues RESTART IDENTITY")
		ts.Close()
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	return ts, cleanup
}

// Test CreateQueue function - main happy path
func TestIntegration_CreateQueue(t *testing.T) {
	ts, cleanup := setupTestServer()
	defer cleanup()

	createBody := []byte(`{"name": "test-queue"}`)
	resp, err := ts.Client().Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
	assert.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

// Test GetQueueList function - all scenarios
func TestIntegration_GetQueueList(t *testing.T) {
	ts, cleanup := setupTestServer()
	defer cleanup()

	t.Run("POSITIVE-BasicRetrieval_ReturnsCreatedQueue", func(t *testing.T) {
		client := ts.Client()

		// Setup: create a queue first
		createBody := []byte(`{"name": "test-queue"}`)
		createResp, _ := client.Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		createResp.Body.Close()

		// Test: get list
		resp, err := client.Get(ts.URL + "/api/v1/queues")
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var ql queueResp
		err = json.NewDecoder(resp.Body).Decode(&ql)
		assert.NoError(t, err)

		assert.Len(t, ql.Data, 1)
		assert.Equal(t, "test-queue", ql.Data[0].Name)
	})
}

// Test DeleteQueueByID function - main happy path
func TestIntegration_DeleteQueueByID(t *testing.T) {
	ts, cleanup := setupTestServer()
	defer cleanup()

	client := ts.Client()

	// Setup: create a queue first
	createBody := []byte(`{"name": "test-queue"}`)
	createResp, _ := client.Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
	createResp.Body.Close()

	// Get queue ID
	listResp, _ := client.Get(ts.URL + "/api/v1/queues")
	var ql queueResp
	json.NewDecoder(listResp.Body).Decode(&ql)
	listResp.Body.Close()
	queueID := ql.Data[0].ID

	// Test: delete queue
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/queues/%d", ts.URL, queueID), nil)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify: queue should be deleted
	verifyResp, _ := client.Get(ts.URL + "/api/v1/queues")
	var verifyQl queueResp
	json.NewDecoder(verifyResp.Body).Decode(&verifyQl)
	verifyResp.Body.Close()

	assert.Empty(t, verifyQl.Data)
}

// Test CreateQueue error handling - primary error path
func TestIntegration_CreateQueue_InvalidJSON(t *testing.T) {
	ts, cleanup := setupTestServer()
	defer cleanup()

	resp, err := ts.Client().Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader([]byte("invalid json")))
	assert.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// Test ProcessQueue function - simple trigger test
func TestIntegration_ProcessQueue(t *testing.T) {
	ts, cleanup := setupTestServer()
	defer cleanup()

	t.Run("POSITIVE-TriggerProcessing_ReturnsSuccess", func(t *testing.T) {
		client := ts.Client()

		createBody := []byte(`{"name": "test-queue"}`)
		resp, err := ts.Client().Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Test: trigger processing (no body needed)
		resp, err = client.Post(ts.URL+"/api/v1/queues/process", "application/json", bytes.NewReader(createBody))
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		resp, err = client.Get(ts.URL + "/api/v1/queues")
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var ql queueResp
		err = json.NewDecoder(resp.Body).Decode(&ql)
		assert.NoError(t, err)

		assert.Len(t, ql.Data, 1)
		assert.Equal(t, "test-queue", ql.Data[0].Name)
		assert.Equal(t, StatusProcessing, ql.Data[0].Status)
	})
}

// RabbitMQ test helper for end-to-end testing
func setupRabbitMQTest(t *testing.T) (*amqp091.Connection, *amqp091.Channel, string, func()) {
	conn, err := amqp091.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		t.Skipf("RabbitMQ not available for testing: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		t.Fatalf("failed to open channel: %v", err)
	}

	// Use the same queue name as processor
	queueName := "queue_processing"

	// Declare queue (idempotent)
	_, err = ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Purge existing messages
	_, err = ch.QueuePurge(queueName, false)
	if err != nil {
		ch.Close()
		conn.Close()
		t.Fatalf("failed to purge queue: %v", err)
	}

	cleanup := func() {
		ch.Close()
		conn.Close()
	}

	return conn, ch, queueName, cleanup
}

// Test ProcessQueue with actual RabbitMQ integration
func TestIntegration_ProcessQueue_WithRabbitMQ(t *testing.T) {
	t.Run("POSITIVE-TriggerProcessing_PublishesToRabbitMQ", func(t *testing.T) {
		// Setup RabbitMQ test environment
		_, ch, queueName, rabbitCleanup := setupRabbitMQTest(t)
		defer rabbitCleanup()

		// Setup HTTP test server
		ts, cleanup := setupTestServer()
		defer cleanup()

		client := ts.Client()

		// Create test queue in database
		createBody := []byte(`{"name": "test-queue-rabbitmq"}`)
		createResp, _ := client.Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		createResp.Body.Close()

		// Trigger processing
		processBody := []byte(`{"name": "test-queue-rabbitmq"}`)
		resp, err := client.Post(ts.URL+"/api/v1/queues/process", "application/json", bytes.NewReader(processBody))
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify message was published to RabbitMQ
		msgs, err := ch.Consume(
			queueName, // queue
			"",        // consumer
			true,      // auto-ack
			false,     // exclusive
			false,     // no-local
			false,     // no-wait
			nil,       // args
		)
		assert.NoError(t, err)

		// Wait for message
		select {
		case msg := <-msgs:
			var message map[string]string
			err := json.Unmarshal(msg.Body, &message)
			assert.NoError(t, err)
			assert.Equal(t, TriggerStartProcessing, message["trigger"])
			t.Logf("Successfully received message: %+v", message)
		case <-time.After(1 * time.Second):
			t.Fatal("message not received from RabbitMQ within 1 seconds")
		}
	})
}

// Test processor eligible queue logic with RabbitMQ
func TestIntegration_ProcessEligibleQueue_WithRabbitMQ(t *testing.T) {
	t.Run("POSITIVE-ProcessEligibleQueue_UpdatesStatus", func(t *testing.T) {
		// Setup RabbitMQ test environment
		_, ch, queueName, rabbitCleanup := setupRabbitMQTest(t)
		defer rabbitCleanup()

		// Setup test database
		cfg := Load()
		db := NewGORM(cfg)
		defer func() {
			db.Exec("DELETE FROM queues")
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}()
		// Create test data
		repo := NewRepository(db)
		testQueue := &Queue{
			Name:   "test-eligible-queue",
			Status: StatusPending,
		}
		err := repo.Save(context.Background(), testQueue)
		assert.NoError(t, err)

		// Publish trigger message to RabbitMQ
		message := map[string]string{"trigger": TriggerStartProcessing}
		body, _ := json.Marshal(message)
		err = ch.Publish(
			"",        // exchange
			queueName, // routing key
			false,     // mandatory
			false,     // immediate
			amqp091.Publishing{
				ContentType: "application/json",
				Body:        body,
			},
		)
		assert.NoError(t, err)

		// Create processor and process one message manually
		processor := NewQueueProcessor(cfg, db)

		// Simulate message consumption and processing
		msgs, err := ch.Consume(
			queueName, // queue
			"",        // consumer
			false,     // auto-ack (manual ack for control)
			false,     // exclusive
			false,     // no-local
			false,     // no-wait
			nil,       // args
		)
		assert.NoError(t, err)

		// Wait for message and process it
		select {
		case d := <-msgs:
			// Manually trigger the processing logic
			ctx := context.Background()
			err := processor.ProcessEligibleQueue(ctx)
			assert.NoError(t, err)

			// Acknowledge message
			d.Ack(false)

		case <-time.After(1 * time.Second):
			t.Fatal("message not received for processing")
		}

		// Verify database changes
		var updatedQueue Queue
		err = db.First(&updatedQueue, testQueue.ID).Error
		assert.NoError(t, err)

		// Status should be either Completed or Failed (random result)
		assert.Contains(t, []QueueStatus{StatusCompleted, StatusFailed}, updatedQueue.Status)

		if updatedQueue.Status == StatusFailed {
			assert.True(t, updatedQueue.LastRetryAt.Valid)
			assert.Equal(t, 1, updatedQueue.RetryCount)
		}
	})
}

// Test shifting queue mechanism and anti-starvation behavior
func TestIntegration_ShiftingQueueMechanism(t *testing.T) {
	t.Run("POSITIVE-ShiftingQueue_AntiStarvation", func(t *testing.T) {
		// Setup test database
		cfg := Load()
		db := NewGORM(cfg)
		defer func() {
			db.Exec("DELETE FROM queues")
			sqlDB, _ := db.DB()
			sqlDB.Close()
		}()

		repo := NewRepository(db)

		// Create multiple queues with different timestamps
		baseTime := time.Now().Add(-1 * time.Hour)

		// Queue 1: Oldest, will be processed first
		queue1 := &Queue{
			Name:   "queue-oldest",
			Status: StatusPending,
		}
		queue1.CreatedAt = baseTime
		err := repo.Save(context.Background(), queue1)
		assert.NoError(t, err)

		// Queue 2: Middle, created later
		queue2 := &Queue{
			Name:   "queue-middle",
			Status: StatusPending,
		}
		queue2.CreatedAt = baseTime.Add(30 * time.Minute)
		err = repo.Save(context.Background(), queue2)
		assert.NoError(t, err)

		// Queue 3: Newest, created last
		queue3 := &Queue{
			Name:   "queue-newest",
			Status: StatusPending,
		}
		queue3.CreatedAt = baseTime.Add(60 * time.Minute)
		err = repo.Save(context.Background(), queue3)
		assert.NoError(t, err)

		// Step 1: First processing should get oldest queue
		queues, err := repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Len(t, queues, 1)
		assert.Equal(t, "queue-oldest", queues[0].Name)

		// Simulate queue1 failure - it should get last_retry_at updated
		queue1.Status = StatusFailed
		queue1.LastRetryAt = sql.NullTime{Time: time.Now(), Valid: true}
		queue1.RetryCount = 1
		err = repo.Save(context.Background(), queue1)
		assert.NoError(t, err)

		// Step 2: Next processing should get queue2 (middle), not the failed queue1
		queues, err = repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Len(t, queues, 1)
		assert.Equal(t, "queue-middle", queues[0].Name)

		// Process queue2 successfully
		queue2.Status = StatusCompleted
		err = repo.Save(context.Background(), queue2)
		assert.NoError(t, err)

		// Step 3: Next processing should get queue3 (newest)
		queues, err = repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Len(t, queues, 1)
		assert.Equal(t, "queue-newest", queues[0].Name)

		// Process queue3 successfully
		queue3.Status = StatusCompleted
		err = repo.Save(context.Background(), queue3)
		assert.NoError(t, err)

		// Step 4: Now only failed queue1 should be eligible (anti-starvation)
		queues, err = repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Len(t, queues, 1)
		assert.Equal(t, "queue-oldest", queues[0].Name)
		assert.Equal(t, StatusFailed, queues[0].Status)
		assert.True(t, queues[0].LastRetryAt.Valid)
		assert.Equal(t, 1, queues[0].RetryCount)

		t.Logf("Shifting queue anti-starvation working correctly")
	})

	t.Run("POSITIVE-NewQueueVsFailedQueue_ProperOrdering", func(t *testing.T) {
		// Setup test database
		cfg := Load()
		db := NewGORM(cfg)
		defer func() {
			db.Exec("DELETE FROM queues")
			sqlDB, _ := db.DB()
			sqlDB.Close()
		}()

		repo := NewRepository(db)

		// Create old failed queue
		baseTime := time.Now().Add(-2 * time.Hour)
		failedQueue := &Queue{
			Name:        "queue-failed-old",
			Status:      StatusFailed,
			LastRetryAt: sql.NullTime{Time: baseTime.Add(1 * time.Hour), Valid: true}, // Failed 1 hour ago
			RetryCount:  1,
		}
		failedQueue.CreatedAt = baseTime
		err := repo.Save(context.Background(), failedQueue)
		assert.NoError(t, err)

		// Wait a moment to ensure different timestamp
		time.Sleep(10 * time.Millisecond)

		// Create new pending queue
		newQueue := &Queue{
			Name:   "queue-new",
			Status: StatusPending,
		}
		// This will have current timestamp (newer than failed queue's last_retry_at)
		err = repo.Save(context.Background(), newQueue)
		assert.NoError(t, err)

		// The failed queue should be processed first because:
		// failed_queue: GREATEST(old_created_at, last_retry_at_1_hour_ago) = last_retry_at_1_hour_ago (older)
		// new_queue: GREATEST(current_created_at, NULL) = current_created_at (newer)
		// ASC ordering means older timestamps get processed first
		queues, err := repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Len(t, queues, 1)
		assert.Equal(t, "queue-failed-old", queues[0].Name) // Older timestamp processed first

		t.Logf("Queue ordering works correctly: older timestamps processed first (anti-starvation)")
	})

	t.Run("POSITIVE-MultipleFailures_RetryCountIncrement", func(t *testing.T) {
		// Setup test database
		cfg := Load()
		db := NewGORM(cfg)
		defer func() {
			db.Exec("DELETE FROM queues")
			sqlDB, _ := db.DB()
			sqlDB.Close()
		}()

		repo := NewRepository(db)

		// Create queue
		queue := &Queue{
			Name:   "queue-multiple-failures",
			Status: StatusPending,
		}
		err := repo.Save(context.Background(), queue)
		assert.NoError(t, err)

		// Simulate multiple failures
		for i := 1; i <= 3; i++ {
			// Get eligible queue
			queues, err := repo.GetEligibleQueues(context.Background())
			assert.NoError(t, err)
			assert.Len(t, queues, 1)
			assert.Equal(t, "queue-multiple-failures", queues[0].Name)

			// Simulate failure
			queue := &queues[0]
			queue.Status = StatusFailed
			queue.LastRetryAt = sql.NullTime{Time: time.Now(), Valid: true}
			queue.RetryCount = i
			err = repo.Save(context.Background(), queue)
			assert.NoError(t, err)

			// Verify retry count
			updatedQueue, err := repo.GetQueueByName(context.Background(), "queue-multiple-failures")
			assert.NoError(t, err)
			assert.Equal(t, i, updatedQueue.RetryCount)
			assert.True(t, updatedQueue.LastRetryAt.Valid)

			t.Logf("Failure %d: retry_count=%d, last_retry_at updated", i, updatedQueue.RetryCount)

			// Small delay to ensure different timestamps
			time.Sleep(10 * time.Millisecond)
		}

		t.Logf("Multiple failures properly increment retry count and update last_retry_at")
	})

	t.Run("POSITIVE-AntiStarvation_FailedQueueGetsRetry", func(t *testing.T) {
		// This test demonstrates the anti-starvation scenario:
		// A queue fails processing, but new queues keep coming.
		// The failed queue should still get a chance to be processed eventually.

		// Setup test database
		cfg := Load()
		db := NewGORM(cfg)
		defer func() {
			db.Exec("DELETE FROM queues")
			sqlDB, _ := db.DB()
			sqlDB.Close()
		}()

		repo := NewRepository(db)

		// Step 1: Create initial queue that will fail
		baseTime := time.Now().Add(-2 * time.Hour)

		oldQueue := &Queue{
			Name:   "queue-will-fail",
			Status: StatusPending,
		}
		oldQueue.CreatedAt = baseTime
		err := repo.Save(context.Background(), oldQueue)
		assert.NoError(t, err)

		// Step 2: Process and fail the old queue
		queues, err := repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "queue-will-fail", queues[0].Name)

		// Simulate failure with last_retry_at set to 1 hour ago
		oldQueue.Status = StatusFailed
		oldQueue.LastRetryAt = sql.NullTime{Time: baseTime.Add(1 * time.Hour), Valid: true}
		oldQueue.RetryCount = 1
		err = repo.Save(context.Background(), oldQueue)
		assert.NoError(t, err)

		// Step 3: Multiple new queues arrive (simulating busy system)
		newQueueNames := []string{"new-queue-1", "new-queue-2", "new-queue-3"}
		for i, name := range newQueueNames {
			newQueue := &Queue{
				Name:   name,
				Status: StatusPending,
			}
			// Each new queue gets progressively newer timestamp
			newQueue.CreatedAt = time.Now().Add(time.Duration(i) * time.Minute)
			err = repo.Save(context.Background(), newQueue)
			assert.NoError(t, err)
		}

		// Step 4: Verify processing order - failed queue should come first
		// because its last_retry_at (1 hour ago) is older than new queues' created_at
		expectedOrder := []string{"queue-will-fail", "new-queue-1", "new-queue-2", "new-queue-3"}

		for i, expectedName := range expectedOrder {
			queues, err := repo.GetEligibleQueues(context.Background())
			assert.NoError(t, err)
			assert.Len(t, queues, 1)
			assert.Equal(t, expectedName, queues[0].Name,
				"Processing order incorrect at step %d", i+1)

			// Mark as completed to move to next
			queue := &queues[0]
			queue.Status = StatusCompleted
			err = repo.Save(context.Background(), queue)
			assert.NoError(t, err)

			t.Logf("Step %d: Processed queue '%s' correctly", i+1, expectedName)
		}

		// Step 5: Verify no more eligible queues
		queues, err = repo.GetEligibleQueues(context.Background())
		assert.NoError(t, err)
		assert.Len(t, queues, 0)

		t.Logf("Anti-starvation mechanism working: failed queue got retry despite new queues arriving")
	})
}
