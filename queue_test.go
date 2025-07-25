package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type queueResp struct {
	Data []Queue `json:"data"`
}

func openInMemorySQLiteWithGreatest() *gorm.DB {
	rawConn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	driver := rawConn.Driver().(*sqlite3.SQLiteDriver)
	driver.ConnectHook = func(conn *sqlite3.SQLiteConn) error {
		return conn.RegisterFunc("GREATEST", func(a, b string) int64 {
			parse := func(s string) int64 {
				if s == "" || s == "0001-01-01 00:00:00" || s == "<nil>" {
					return 0
				}
				formats := []string{
					"2006-01-02 15:04:05.999999999-07:00",
					"2006-01-02 15:04:05-07:00",
					"2006-01-02 15:04:05",
				}
				for _, f := range formats {
					t, err := time.Parse(f, s)
					if err == nil {
						return t.UnixNano()
					}
					log.Info().Err(err).Msgf("parsed time: %v", t)
				}
				return 0
			}
			ta := parse(a)
			tb := parse(b)
			if ta > tb {
				return ta
			}
			return tb
		}, true)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: rawConn}, &gorm.Config{
		Logger: NewLogLevel("info"),
	})
	if err != nil {
		panic(err)
	}
	return db
}

func setupTestDatabase(t *testing.T) (*gorm.DB, func()) {
	db := openInMemorySQLiteWithGreatest()
	if err := Migrate(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
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
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	setEnv(t, map[string]string{
		"DATABASE_HOST":      "localhost",
		"DATABASE_PORT":      "5432",
		"DATABASE_USER":      "user",
		"DATABASE_PASSWORD":  "pass",
		"DATABASE_NAME":      "dbname",
		"RABBITMQ_URL":       "amqp://guest:guest@localhost:5672/",
		"DATABASE_LOG_LEVEL": "info",
	})
	db, cleanupDB := setupTestDatabase(t)
	ts, cleanupTs := setupTestServerWithDB(t, db)
	cleanup := func() {
		cleanupTs()
		cleanupDB()
	}
	return ts, cleanup
}

// Helper untuk setup test server dengan DB custom (tanpa close DB di cleanup)
func setupTestServerWithDB(t *testing.T, db *gorm.DB) (*httptest.Server, func()) {
	setEnv(t, map[string]string{
		"DATABASE_HOST":      "localhost",
		"DATABASE_PORT":      "5432",
		"DATABASE_USER":      "user",
		"DATABASE_PASSWORD":  "pass",
		"DATABASE_NAME":      "dbname",
		"RABBITMQ_URL":       "amqp://guest:guest@localhost:5672/",
		"DATABASE_LOG_LEVEL": "info",
	})
	mockCloser := NewMockCloser(t)
	mockPublisher := NewMockPublisher(t)
	mockPublisher.EXPECT().QueueDeclare(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(amqp091.Queue{}, nil).Maybe()
	mockPublisher.EXPECT().PublishWithContext(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	processor, err := NewQueueProcessor(nil, db, &FakeAPI{}, WithConnection(mockCloser), WithChannel(mockPublisher))
	assert.NoError(t, err)
	server, err := NewServer(WithDB(db), WithProcessor(processor))
	assert.NoError(t, err)
	ts := httptest.NewServer(server.router)
	cleanup := func() {
		ts.Close()
	}
	return ts, cleanup
}

// Test CreateQueue function - main happy path
func TestIntegration_CreateQueue(t *testing.T) {
	t.Run("POSITIVE-CreateQueue_ReturnsCreated", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		createBody := []byte(`{"name": "test-queue"}`)
		resp, err := ts.Client().Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("NEGATIVE-DBError_Returns500", func(t *testing.T) {
		db, shutdownDB := setupTestDatabase(t)
		ts, cleanup := setupTestServerWithDB(t, db)
		shutdownDB()
		defer cleanup()
		createBody := []byte(`{"name": "test-queue"}`)
		resp, err := ts.Client().Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("NEGATIVE-InvalidJSON_ReturnsError", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()

		resp, err := ts.Client().Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader([]byte("invalid json")))
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// Test GetQueueList function - all scenarios
func TestIntegration_GetQueueList(t *testing.T) {
	t.Run("POSITIVE-BasicRetrieval_ReturnsCreatedQueue", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		client := ts.Client()
		createBody := []byte(`{"name": "test-queue"}`)
		createResp, _ := client.Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		createResp.Body.Close()
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

	t.Run("NEGATIVE-DBError_Returns500", func(t *testing.T) {
		db, shutdownDB := setupTestDatabase(t)
		ts, cleanup := setupTestServerWithDB(t, db)
		shutdownDB()
		defer cleanup()
		client := ts.Client()
		resp, err := client.Get(ts.URL + "/api/v1/queues")
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// Test DeleteQueueByID function - main happy path
func TestIntegration_DeleteQueueByID(t *testing.T) {
	t.Run("POSITIVE-DeleteQueue_ReturnsOK", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		client := ts.Client()
		createBody := []byte(`{"name": "test-queue"}`)
		createResp, _ := client.Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		createResp.Body.Close()
		listResp, _ := client.Get(ts.URL + "/api/v1/queues")
		var ql queueResp
		json.NewDecoder(listResp.Body).Decode(&ql)
		listResp.Body.Close()
		queueID := ql.Data[0].ID
		req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/queues/%d", ts.URL, queueID), nil)
		resp, err := client.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		verifyResp, _ := client.Get(ts.URL + "/api/v1/queues")
		var verifyQl queueResp
		json.NewDecoder(verifyResp.Body).Decode(&verifyQl)
		verifyResp.Body.Close()
		assert.Empty(t, verifyQl.Data)
	})

	t.Run("NEGATIVE-NotFound_Returns500", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		client := ts.Client()
		req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/queues/%d", ts.URL, 99999), nil)
		resp, err := client.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("NEGATIVE-InvalidID_Returns500", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		client := ts.Client()
		req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/queues/invalid", nil)
		resp, err := client.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("NEGATIVE-DBError_Returns500", func(t *testing.T) {
		db, shutdownDB := setupTestDatabase(t)
		ts, cleanup := setupTestServerWithDB(t, db)
		defer cleanup()
		client := ts.Client()
		createBody := []byte(`{"name": "test-queue"}`)
		createResp, _ := client.Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		createResp.Body.Close()
		listResp, _ := client.Get(ts.URL + "/api/v1/queues")
		var ql queueResp
		json.NewDecoder(listResp.Body).Decode(&ql)
		listResp.Body.Close()
		queueID := ql.Data[0].ID
		shutdownDB()
		req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/queues/%d", ts.URL, queueID), nil)
		resp, err := client.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

func TestIntegration_LeaveQueue(t *testing.T) {
	t.Run("POSITIVE-LeaveQueue_ReturnsOK", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		client := ts.Client()
		createBody := []byte(`{"name": "leave-queue"}`)
		client.Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		leaveBody := []byte(`{"name": "leave-queue"}`)
		resp, err := client.Post(ts.URL+"/api/v1/queues/leave", "application/json", bytes.NewReader(leaveBody))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		verifyResp, _ := client.Get(ts.URL + "/api/v1/queues")
		var verifyQl queueResp
		json.NewDecoder(verifyResp.Body).Decode(&verifyQl)
		verifyResp.Body.Close()
		assert.Empty(t, verifyQl.Data)
	})

	t.Run("NEGATIVE-NotFound_ReturnsNon200", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		client := ts.Client()
		leaveBody := []byte(`{"name": "not-exist"}`)
		resp, err := client.Post(ts.URL+"/api/v1/queues/leave", "application/json", bytes.NewReader(leaveBody))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("NEGATIVE-DBError_Returns500", func(t *testing.T) {
		db, shutdownDB := setupTestDatabase(t)
		ts, cleanup := setupTestServerWithDB(t, db)
		defer cleanup()
		client := ts.Client()
		createBody := []byte(`{"name": "leave-queue"}`)
		client.Post(ts.URL+"/api/v1/queues", "application/json", bytes.NewReader(createBody))
		shutdownDB()
		leaveBody := []byte(`{"name": "leave-queue"}`)
		resp, err := client.Post(ts.URL+"/api/v1/queues/leave", "application/json", bytes.NewReader(leaveBody))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("NEGATIVE-InvalidJSON_Returns500", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		client := ts.Client()
		resp, err := client.Post(ts.URL+"/api/v1/queues/leave", "application/json", bytes.NewReader([]byte("invalid json")))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("NEGATIVE-TriggerProcessingError_Returns500", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DATABASE_HOST":      "localhost",
			"DATABASE_PORT":      "5432",
			"DATABASE_USER":      "user",
			"DATABASE_PASSWORD":  "pass",
			"DATABASE_NAME":      "dbname",
			"RABBITMQ_URL":       "amqp://guest:guest@localhost:5672/",
			"DATABASE_LOG_LEVEL": "info",
		})
		db, cleanupDB := setupTestDatabase(t)
		defer cleanupDB()
		// Insert queue dulu
		db.Create(&Queue{Name: "leave-queue"})
		// Mock processor
		mockProcessor := NewMockQueueTrigger(t)
		mockProcessor.EXPECT().TriggerProcessing(mock.Anything).Return(assert.AnError).Once()
		server, err := NewServer(WithDB(db), WithProcessor(mockProcessor))
		assert.NoError(t, err)
		ts := httptest.NewServer(server.router)
		defer ts.Close()
		client := ts.Client()
		leaveBody := []byte(`{"name": "leave-queue"}`)
		resp, err := client.Post(ts.URL+"/api/v1/queues/leave", "application/json", bytes.NewReader(leaveBody))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// Test ProcessQueue function - simple trigger test
func TestIntegration_ProcessQueue(t *testing.T) {
	t.Run("POSITIVE-TriggerProcessing_ReturnsSuccess", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
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

	// NEGATIVE: invalid JSON
	t.Run("NEGATIVE-InvalidJSON_Returns500", func(t *testing.T) {
		ts, cleanup := setupTestServer(t)
		defer cleanup()
		resp, err := ts.Client().Post(ts.URL+"/api/v1/queues/process", "application/json", bytes.NewReader([]byte("invalid json")))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	// NEGATIVE: DB error saat GetQueueByName
	t.Run("NEGATIVE-DBError_GetQueueByName_Returns500", func(t *testing.T) {
		db, shutdownDB := setupTestDatabase(t)
		ts, cleanup := setupTestServerWithDB(t, db)
		shutdownDB()
		defer cleanup()
		body := []byte(`{"name": "test-queue"}`)
		resp, err := ts.Client().Post(ts.URL+"/api/v1/queues/process", "application/json", bytes.NewReader(body))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	// NEGATIVE: DB error saat Save (setelah GetQueueByName sukses)
	t.Run("NEGATIVE-DBError_Save_Returns500", func(t *testing.T) {
		db, shutdownDB := setupTestDatabase(t)
		// Insert queue dulu
		db.Create(&Queue{Name: "test-queue"})
		ts, cleanup := setupTestServerWithDB(t, db)
		defer cleanup()
		shutdownDB()
		body := []byte(`{"name": "test-queue"}`)
		resp, err := ts.Client().Post(ts.URL+"/api/v1/queues/process", "application/json", bytes.NewReader(body))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	// NEGATIVE: error saat TriggerProcessing (mock processor)
	t.Run("NEGATIVE-TriggerProcessingError_Returns500", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DATABASE_HOST":      "localhost",
			"DATABASE_PORT":      "5432",
			"DATABASE_USER":      "user",
			"DATABASE_PASSWORD":  "pass",
			"DATABASE_NAME":      "dbname",
			"RABBITMQ_URL":       "amqp://guest:guest@localhost:5672/",
			"DATABASE_LOG_LEVEL": "info",
		})
		db, cleanupDB := setupTestDatabase(t)
		defer cleanupDB()
		// Insert queue dulu
		db.Create(&Queue{Name: "test-queue"})
		// Mock processor
		mockProcessor := NewMockQueueTrigger(t)
		mockProcessor.EXPECT().TriggerProcessing(mock.Anything).Return(assert.AnError).Once()
		server, err := NewServer(WithDB(db), WithProcessor(mockProcessor))
		assert.NoError(t, err)
		ts := httptest.NewServer(server.router)
		defer ts.Close()
		body := []byte(`{"name": "test-queue"}`)
		resp, err := ts.Client().Post(ts.URL+"/api/v1/queues/process", "application/json", bytes.NewReader(body))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
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
		ts, cleanup := setupTestServer(t)
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
		setEnv(t, map[string]string{
			"DATABASE_HOST":      "localhost",
			"DATABASE_PORT":      "5432",
			"DATABASE_USER":      "user",
			"DATABASE_PASSWORD":  "pass",
			"DATABASE_NAME":      "dbname",
			"RABBITMQ_URL":       "amqp://guest:guest@localhost:5672/",
			"DATABASE_LOG_LEVEL": "info",
		})
		cfg, err := Load()
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
		db, cleanup := setupTestDatabase(t)
		defer cleanup()
		// Create test data
		repo := NewRepository(db)
		testQueue := &Queue{
			Name:   "test-eligible-queue",
			Status: StatusPending,
		}
		err = repo.Save(context.Background(), testQueue)
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
		processor, err := NewQueueProcessor(cfg, db, apiMock)
		assert.NoError(t, err)
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

func TestQueueStatus(t *testing.T) {
	assert.Equal(t, "completed", StatusCompleted.String())
	assert.Equal(t, "failed", StatusFailed.String())
	assert.Equal(t, "pending", StatusPending.String())
	assert.Equal(t, "processing", StatusProcessing.String())
	assert.Equal(t, "unknown", QueueStatus(666).String())
}

func TestWriteJSON_ErrorHandling(t *testing.T) {
	t.Run("NEGATIVE-MarshalError_Returns500AndLogs", func(t *testing.T) {
		// Create a mock HTTP response recorder
		rr := httptest.NewRecorder()

		// Create a data structure that cannot be marshaled to JSON (e.g., a channel)
		unmarshalableData := make(chan int)

		// Capture logs
		var buf bytes.Buffer
		originalLogger := log.Logger
		log.Logger = zerolog.New(&buf)
		defer func() {
			log.Logger = originalLogger // Reset log output
		}()

		// Call WriteJSON with the unmarshalable data
		WriteJSON(rr, http.StatusOK, unmarshalableData)

		// Assertions
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		// Verify response body
		expectedBody := `{"error":"internal server error"}`
		assert.JSONEq(t, expectedBody, rr.Body.String())

		// Verify log output
		assert.Contains(t, buf.String(), "failed to marshal JSON")
		assert.Contains(t, buf.String(), "error") // Check for error level in log
	})

	t.Run("NEGATIVE-WriteError_LogsError", func(t *testing.T) {
		// Capture log output
		var buf bytes.Buffer
		originalLogger := log.Logger
		log.Logger = zerolog.New(&buf)
		defer func() {
			log.Logger = originalLogger
		}()

		// Use the mock writer that always returns an error
		mockWriter := &errorResponseWriter{}

		// Call the function under test
		WriteJSON(mockWriter, http.StatusOK, nil)

		// Assert that the log contains the expected error message
		assert.Contains(t, buf.String(), "failed to write error response")
	})
}

// Create an error that will cause a marshal error (e.g., containing a channel)
type unmarshalableError struct {
	Err chan int `json:"err"`
}

func (e *unmarshalableError) Error() string {
	return "unmarshalable error"
}

func TestWriteJSONError(t *testing.T) {
	t.Run("POSITIVE-ValidError_Returns500AndJSON", func(t *testing.T) {
		// Create a mock HTTP response recorder
		rr := httptest.NewRecorder()

		// Define a test error
		testError := fmt.Errorf("something went wrong")

		// Call WriteJSONError
		WriteJSONError(rr, testError)

		// Assertions
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		// Verify response body
		b, _ := json.Marshal(map[string]any{"error": testError})
		assert.JSONEq(t, string(b), rr.Body.String())
	})

	t.Run("NEGATIVE-MarshalError_LogsError", func(t *testing.T) {
		// Capture log output
		var buf bytes.Buffer
		originalLogger := log.Logger
		log.Logger = zerolog.New(&buf)
		defer func() {
			log.Logger = originalLogger
		}()

		// Use a mock writer
		mockWriter := httptest.NewRecorder()

		badErr := &unmarshalableError{Err: make(chan int)}

		// Call the function under test
		WriteJSONError(mockWriter, badErr)

		// Assert that the log contains the expected error message
		assert.Contains(t, buf.String(), "failed to marshal JSON for error response")
	})

	t.Run("NEGATIVE-WriteError_LogsError", func(t *testing.T) {
		// Capture log output
		var buf bytes.Buffer
		originalLogger := log.Logger
		log.Logger = zerolog.New(&buf)
		defer func() {
			log.Logger = originalLogger
		}()

		// Use the mock writer that always returns an error
		mockWriter := &errorResponseWriter{}

		// Call the function under test
		WriteJSONError(mockWriter, fmt.Errorf("test error"))

		// Assert that the log contains the expected error message
		assert.Contains(t, buf.String(), "failed to write error response")
	})
}

// errorResponseWriter is a mock ResponseWriter that always fails on Write.
type errorResponseWriter struct {
	header http.Header
}

func (w *errorResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *errorResponseWriter) WriteHeader(statusCode int) {}

func (w *errorResponseWriter) Write(b []byte) (int, error) {
	return 0, fmt.Errorf("forced write error")
}
