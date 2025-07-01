package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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
