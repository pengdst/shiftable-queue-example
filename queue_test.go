package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type queueResp struct {
	Data []Queue `json:"data"`
}

func setupTestDatabase(t *testing.T) (*gorm.DB, func()) {
	db := NewGORM(Load())
	// Pool setup
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}
	Migrate(db)
	cleanup := func() {
		db.Exec("DELETE FROM queues")
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}
	return db, cleanup
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
func TestIntegration_RabbitMQ_QueueProcessing(t *testing.T) {
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
		apiMock := NewMockRestAPI(t)
		processor := NewQueueProcessor(cfg, db, apiMock)
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()

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
		cfg := Load()
		db, cleanup := setupTestDatabase(t)
		defer cleanup()

		repo := NewRepository(db)
		apiMock := NewMockRestAPI(t)
		processor := NewQueueProcessor(cfg, db, apiMock)

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

		queues, _ := repo.Fetch(context.Background())
		t.Logf("[DEBUG] After insert & update created_at: \n%v", queueNames(queues))
		// Print eligible queue sebelum proses
		eligible, _ := repo.GetEligibleQueues(context.Background())
		t.Logf("[DEBUG] Eligible before process: %v", queueNames(eligible))

		// Step 1: Process (should get oldest, fail)
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After 1st process: \n%v", queueNames(queues))

		// Step 2: Process (should get middle, success)
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After 2nd process: \n%v", queueNames(queues))

		// Step 3: Process (should get newest, success)
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After 3rd process: \n%v", queueNames(queues))

		// Step 4: Process (should get failed oldest again, success)
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After 4th process: \n%v", queueNames(queues))

		// Cek status akhir queue di DB
		var q1, q2, q3 Queue
		_ = db.Where("name = ?", "queue-oldest").First(&q1).Error
		_ = db.Where("name = ?", "queue-middle").First(&q2).Error
		_ = db.Where("name = ?", "queue-newest").First(&q3).Error

		assert.Equal(t, StatusCompleted, q1.Status)
		assert.Equal(t, StatusCompleted, q2.Status)
		assert.Equal(t, StatusCompleted, q3.Status)
	})

	t.Run("NEGATIVE-Starvation_BurstInsert", func(t *testing.T) {
		db, cleanup := setupTestDatabase(t)
		defer cleanup()
		repo := NewRepository(db)
		apiMock := NewMockRestAPI(t)
		processor := NewQueueProcessor(Load(), db, apiMock)

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
		queues, _ := repo.Fetch(context.Background())
		t.Logf("After fail queue-fail: \n%v", queueNames(queues))
		// Step 4: Semua queue lain success
		for i := 0; i < 10; i++ {
			apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
			_ = processor.ProcessEligibleQueue(context.Background())
			queues, _ = repo.Fetch(context.Background())
			t.Logf("After process burst-%d: \n%v", i, queueNames(queues))
		}
		// Step 5: Retry queue-fail, success
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After retry queue-fail: \n%v", queueNames(queues))
		// Step 6: Assert queue-fail completed
		var qf Queue
		_ = db.Where("name = ?", "queue-fail").First(&qf).Error
		assert.Equal(t, StatusCompleted, qf.Status)
		assert.Equal(t, 1, qf.RetryCount)
	})

	t.Run("POSITIVE-MultipleFailures_RetryCount", func(t *testing.T) {
		db, cleanup := setupTestDatabase(t)
		defer cleanup()
		repo := NewRepository(db)
		apiMock := NewMockRestAPI(t)
		processor := NewQueueProcessor(Load(), db, apiMock)

		queue := &Queue{Name: "queue-retry", Status: StatusPending}
		_ = repo.Save(context.Background(), queue)
		// Fail 2x
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Twice()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ := repo.Fetch(context.Background())
		t.Logf("After 1st fail: \n%v", queueNames(queues))
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After 2nd fail: \n%v", queueNames(queues))
		// Success
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After success: \n%v", queueNames(queues))
		// Assert
		var qr Queue
		_ = db.Where("name = ?", "queue-retry").First(&qr).Error
		assert.Equal(t, StatusCompleted, qr.Status)
		assert.Equal(t, 2, qr.RetryCount)
	})

	t.Run("POSITIVE-InterleavedSuccessFailure", func(t *testing.T) {
		db, cleanup := setupTestDatabase(t)
		defer cleanup()
		repo := NewRepository(db)
		apiMock := NewMockRestAPI(t)
		processor := NewQueueProcessor(Load(), db, apiMock)

		// Step 1: Insert Queue1 & Queue2
		queue1 := &Queue{Name: "queue1", Status: StatusPending}
		queue2 := &Queue{Name: "queue2", Status: StatusPending}
		_ = repo.Save(context.Background(), queue1)
		_ = repo.Save(context.Background(), queue2)
		// Step 2: Queue1 gagal
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ := repo.Fetch(context.Background())
		t.Logf("After queue1 fail: \n%v", queueNames(queues))
		// Step 3: Queue2 sukses
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After queue2 success: \n%v", queueNames(queues))
		// Step 4: Insert Queue3 setelah proses mulai
		queue3 := &Queue{Name: "queue3", Status: StatusPending}
		_ = repo.Save(context.Background(), queue3)
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After insert queue3: \n%v", queueNames(queues))
		// Step 5: Queue1 retry sukses
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After queue1 retry success: \n%v", queueNames(queues))
		// Step 6: Queue3 sukses
		apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
		_ = processor.ProcessEligibleQueue(context.Background())
		queues, _ = repo.Fetch(context.Background())
		t.Logf("After queue3 success: \n%v", queueNames(queues))
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
	})

	t.Run("POSITIVE-AllQueuesFailedThenSucceed", func(t *testing.T) {
		db, cleanup := setupTestDatabase(t)
		defer cleanup()
		repo := NewRepository(db)
		apiMock := NewMockRestAPI(t)
		processor := NewQueueProcessor(Load(), db, apiMock)

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
		for i := 0; i < 3; i++ {
			apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(assert.AnError).Once()
			_ = processor.ProcessEligibleQueue(context.Background())
			queues, _ := repo.Fetch(context.Background())
			t.Logf("After fail-%d: \n%v", i+1, queueNames(queues))
		}
		// Step 3: Semua retry sukses
		for i := 0; i < 3; i++ {
			apiMock.EXPECT().SimulateProcessing(mock.Anything).Return(nil).Once()
			_ = processor.ProcessEligibleQueue(context.Background())
			queues, _ := repo.Fetch(context.Background())
			t.Logf("After retry success-%d: \n%v", i+1, queueNames(queues))
		}
		// Assert status akhir dan retry count
		for _, name := range []string{"fail1", "fail2", "fail3"} {
			var q Queue
			_ = db.Where("name = ?", name).First(&q).Error
			assert.Equal(t, StatusCompleted, q.Status)
			assert.Equal(t, 1, q.RetryCount)
		}
	})
}

func queueNames(queues []Queue) string {
	names := make([]string, len(queues))
	for i, q := range queues {
		names[i] = fmt.Sprintf("%d. Queue %s (created_at=%v, last_retry_at=%v, retry=%d, status=%v)", i+1, q.Name, q.CreatedAt.Format(time.TimeOnly), q.LastRetryAt.Time.Format(time.TimeOnly), q.RetryCount, q.Status)
	}
	return strings.Join(names, "\n")
}
